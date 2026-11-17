import { devices, type Page } from '@playwright/test'
import { expect, localDaysAgo, swipe, test, type Account, type Core, type Ledger } from './fixtures.ts'

// History from 700 days back, so "All" is monthly; a second currency, so two charts
async function seedHistory(page: Page, core: Core, { workspace, account }: Ledger): Promise<Account> {
  const operations = `/workspaces/${workspace.id}/operations`
  await core.post(operations, {
    amount: 100,
    kind: 'INCOME',
    accountId: account.id,
    occurredAt: await localDaysAgo(page, 700),
  })
  await core.post(operations, {
    amount: 30,
    kind: 'EXPENSE',
    accountId: account.id,
    occurredAt: await localDaysAgo(page, 200),
  })
  const dollars = await core.post<Account>(`/workspaces/${workspace.id}/accounts`, {
    name: 'Dollars',
    currency: 'USD',
  })
  await core.post(operations, {
    amount: 50,
    kind: 'INCOME',
    accountId: dollars.id,
    occurredAt: await localDaysAgo(page, 300),
  })
  return dollars
}

// The granularity leads the line of hints beside the presets
const granularity = (page: Page, label: string) => page.getByText(new RegExp(`^${label} ·`))

test('presets switch the granularity, one chart per currency', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}/charts`)

  await expect(page.getByRole('region', { name: 'EUR' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'USD' })).toBeVisible()
  await expect(granularity(page, 'Monthly')).toBeVisible()

  const presets = page.getByRole('group', { name: 'Range' })
  await presets.getByRole('button', { name: '3M' }).click()
  await expect(granularity(page, 'Daily')).toBeVisible()
  await expect(presets.getByRole('button', { name: '3M' })).toHaveAttribute('aria-pressed', 'true')

  await presets.getByRole('button', { name: '1Y' }).click()
  await expect(granularity(page, 'Weekly')).toBeVisible()

  await presets.getByRole('button', { name: 'All' }).click()
  await expect(granularity(page, 'Monthly')).toBeVisible()
})

test('picking one account leaves its currency alone', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}/charts`)
  await expect(page.getByRole('region', { name: 'EUR' })).toBeVisible()

  await page.getByRole('button', { name: 'Account' }).click()
  await page.getByRole('menuitem', { name: 'Dollars' }).click()

  await expect(page.getByRole('region', { name: 'USD' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'EUR' })).toHaveCount(0)
})

test('a sideways swipe across a chart picks a range', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}/charts`)
  const chart = page.getByRole('region', { name: 'EUR' }).locator('canvas').first()
  await expect(granularity(page, 'Monthly')).toBeVisible()

  // The swipe starts at 15 % of the chart and runs 45 % of its width: about a year of the 700 days, so weekly
  const box = (await chart.boundingBox())!
  await swipe(page, chart, 'right', { distance: box.width * 0.45 })

  await expect(granularity(page, 'Weekly')).toBeVisible()
})

test('a write elsewhere refreshes the charts on return', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}/charts`)
  await expect(page.getByRole('region', { name: 'EUR' })).toBeVisible()

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('5')
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  // Back through the app's history, not a reload: the write invalidated the cached series, so it is read again
  const reread = page.waitForResponse((response) => response.url().includes('/statistics/balances/series'))
  await page.goBack()
  expect((await reread).status()).toBe(200)
  await expect(page.getByRole('region', { name: 'EUR' })).toBeVisible()
})

// The desktop profile's screen and input, without its browser type, which a describe group may not change
const { viewport, userAgent, deviceScaleFactor, isMobile, hasTouch } = devices['Desktop Chrome']

test.describe('with a mouse', () => {
  test.use({ viewport, userAgent, deviceScaleFactor, isMobile, hasTouch })

  test('a drag across a chart picks a range', async ({ page, core, ledger }) => {
    await seedHistory(page, core, ledger)
    await page.goto(`/w/${ledger.workspace.id}/charts`)
    const chart = page.getByRole('region', { name: 'EUR' }).locator('canvas').first()
    await expect(granularity(page, 'Monthly')).toBeVisible()

    const box = (await chart.boundingBox())!
    const y = box.y + box.height * 0.4
    await page.mouse.move(box.x + box.width * 0.5, y)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width * 0.95, y, { steps: 10 })
    await page.mouse.up()

    await expect(granularity(page, 'Weekly')).toBeVisible()
  })
})
