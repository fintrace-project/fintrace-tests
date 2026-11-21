import type { Page, Response } from '@playwright/test'
import { expect, localDaysAgo, test, type Account, type Core, type Ledger } from './fixtures.ts'

// History from 700 days back in EUR, and a second currency's account, so the page has two to choose from
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

// The balance series request that satisfies `matches`, read from its query
function seriesRequest(page: Page, matches: (query: URLSearchParams) => boolean): Promise<Response> {
  return page.waitForResponse((response) => {
    const url = new URL(response.url())
    return url.pathname.endsWith('/statistics/balances/series') && matches(url.searchParams)
  })
}

const chartsOf = (ledger: Ledger) => `/w/${ledger.workspace.id}/charts`
const balanceIn = (page: Page, currency: string) =>
  page.getByRole('heading', { name: `Balance in ${currency}` })

test('the period bar sets the window and the granularity', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  const { year, month, monthName } = await page.evaluate(() => {
    const now = new Date()
    return {
      year: String(now.getFullYear()),
      month: `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`,
      monthName: now.toLocaleDateString('en', { month: 'long' }),
    }
  })

  // This year by default: from its first day
  const yearly = seriesRequest(page, (query) => query.get('from') === `${year}-01-01`)
  await page.goto(chartsOf(ledger))
  expect((await yearly).status()).toBe(200)
  const bar = page.getByRole('group', { name: 'Period', exact: true })
  await expect(bar.getByRole('button', { name: 'This year' })).toHaveAttribute('aria-pressed', 'true')

  // A month is drawn by day
  const daily = seriesRequest(
    page,
    (query) => query.get('granularity') === 'DAY' && query.get('from') === `${month}-01`,
  )
  await bar.getByRole('button', { name: /pick another month/ }).click()
  await page
    .getByRole('dialog')
    .getByRole('button', { name: new RegExp(`^${monthName} ${year}$`) })
    .click()
  expect((await daily).status()).toBe(200)
  await expect(page).toHaveURL(new RegExp(`period=${month}`))

  await bar.getByRole('button', { name: 'All time' }).click()
  await expect(bar.getByRole('button', { name: 'All time' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page).toHaveURL(/period=all/)
  await expect(balanceIn(page, 'EUR')).toBeVisible()
})

test('one currency at a time, switched by the chips', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(chartsOf(ledger))

  const chips = page.getByRole('group', { name: 'Currency' })
  await expect(chips.getByRole('button', { name: 'EUR' })).toHaveAttribute('aria-pressed', 'true')
  await expect(balanceIn(page, 'EUR')).toBeVisible()

  await chips.getByRole('button', { name: 'USD' }).click()
  await expect(balanceIn(page, 'USD')).toBeVisible()
  await expect(balanceIn(page, 'EUR')).toHaveCount(0)
  await expect(page).toHaveURL(/currency=USD/)
})

// The summary's figures in order: income, expenses, net, saved
const summaryOf = (page: Page) => page.getByRole('group', { name: 'Period summary' }).getByRole('definition')

test("the summary shows the period's income, expenses, net and what was saved", async ({
  page,
  core,
  ledger,
}) => {
  const operations = `/workspaces/${ledger.workspace.id}/operations`
  const now = await localDaysAgo(page, 0)
  await core.post(operations, { amount: 100, kind: 'INCOME', accountId: ledger.account.id, occurredAt: now })
  await core.post(operations, { amount: 30, kind: 'EXPENSE', accountId: ledger.account.id, occurredAt: now })
  await page.goto(chartsOf(ledger))

  await expect(summaryOf(page)).toHaveText([/€100\.00/, /€30\.00/, /€70\.00/, '70%'])
})

test('with no income in the period, Saved is blank', async ({ page, core, ledger }) => {
  await core.post(`/workspaces/${ledger.workspace.id}/operations`, {
    amount: 30,
    kind: 'EXPENSE',
    accountId: ledger.account.id,
    occurredAt: await localDaysAgo(page, 0),
  })
  await page.goto(chartsOf(ledger))

  await expect(summaryOf(page)).toHaveText([/€0\.00/, /€30\.00/, /€30\.00/, '—'])
})

test('with one currency held there is nothing to switch', async ({ page, core, ledger }) => {
  await core.post(`/workspaces/${ledger.workspace.id}/operations`, {
    amount: 10,
    kind: 'INCOME',
    accountId: ledger.account.id,
    occurredAt: await localDaysAgo(page, 10),
  })
  // The chips wait on the currencies held, so their absence means something only once those have arrived
  const held = page.waitForResponse((response) => response.url().includes('/statistics/balances/currencies'))
  await page.goto(chartsOf(ledger))
  expect((await held).status()).toBe(200)

  await expect(balanceIn(page, 'EUR')).toBeVisible()
  await expect(page.getByRole('group', { name: 'Currency' })).toHaveCount(0)
})

test('picking an account draws its currency and hides the chips', async ({ page, core, ledger }) => {
  const dollars = await seedHistory(page, core, ledger)
  await page.goto(chartsOf(ledger))
  await expect(page.getByRole('group', { name: 'Currency' })).toBeVisible()

  const narrowed = seriesRequest(page, (query) => query.get('accountIds') === dollars.id)
  await page.getByRole('button', { name: 'Account' }).click()
  await page.getByRole('menuitem', { name: 'Dollars' }).click()
  expect((await narrowed).status()).toBe(200)

  await expect(balanceIn(page, 'USD')).toBeVisible()
  await expect(page.getByRole('group', { name: 'Currency' })).toHaveCount(0)
  await expect(page).toHaveURL(new RegExp(`account=${dollars.id}`))
})

test('the address keeps the chart across a reload', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(chartsOf(ledger))

  const bar = page.getByRole('group', { name: 'Period', exact: true })
  await bar.getByRole('button', { name: 'All time' }).click()
  await page.getByRole('group', { name: 'Currency' }).getByRole('button', { name: 'USD' }).click()
  await expect(page).toHaveURL(/currency=USD/)

  await page.reload()
  await expect(bar.getByRole('button', { name: 'All time' })).toHaveAttribute('aria-pressed', 'true')
  await expect(
    page.getByRole('group', { name: 'Currency' }).getByRole('button', { name: 'USD' }),
  ).toHaveAttribute('aria-pressed', 'true')
  await expect(balanceIn(page, 'USD')).toBeVisible()

  await page.getByRole('button', { name: 'Account' }).click()
  await page.getByRole('menuitem', { name: 'Wallet' }).click()
  await page.reload()
  await expect(page.getByRole('button', { name: 'Account' })).toHaveText(/Wallet/)
  await expect(balanceIn(page, 'EUR')).toBeVisible()
})

test('a write elsewhere refreshes the chart on return', async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(chartsOf(ledger))
  await expect(balanceIn(page, 'EUR')).toBeVisible()

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('5')
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  // Back through the app's history, not a reload: the write invalidated the cached series, so it is read again
  const reread = seriesRequest(page, () => true)
  await page.goBack()
  expect((await reread).status()).toBe(200)
  await expect(balanceIn(page, 'EUR')).toBeVisible()
})

test('the income and expenses panel follows the period and the account', async ({ page, core, ledger }) => {
  const dollars = await seedHistory(page, core, ledger)
  const year = await page.evaluate(() => String(new Date().getFullYear()))
  const cashflow = (matches: (query: URLSearchParams) => boolean) =>
    page.waitForResponse((response) => {
      const url = new URL(response.url())
      return url.pathname.endsWith('/statistics/cashflow') && matches(url.searchParams)
    })

  const yearly = cashflow((query) => query.get('from') === `${year}-01-01`)
  await page.goto(chartsOf(ledger))
  expect((await yearly).status()).toBe(200)
  const panel = page.locator('section', {
    has: page.getByRole('heading', { name: 'Income and expenses in EUR' }),
  })
  await expect(panel.locator('canvas').first()).toBeVisible()

  const narrowed = cashflow((query) => query.get('accountIds') === dollars.id)
  await page.getByRole('button', { name: 'Account' }).click()
  await page.getByRole('menuitem', { name: 'Dollars' }).click()
  expect((await narrowed).status()).toBe(200)
  await expect(page.getByRole('heading', { name: 'Income and expenses in USD' })).toBeVisible()
})

// The tooltip's figure, inside the balance panel, where ECharts puts it: the axes there are drawn on the canvas,
// and the period summary's amounts are outside it
const tooltipFigure = (page: Page) =>
  page
    .locator('section', { has: balanceIn(page, 'EUR') })
    .getByText(/€\d/)
    .filter({ visible: true })

// A finger through the browser, as the default phone profile's touch screen reports it
// `pause` between moves makes a slow drag; without it every event is sent at once, a flick well inside the 300 ms
// under which ECharts takes a touch for a click
async function touch(page: Page, points: { x: number; y: number }[], pause = 0): Promise<void> {
  const cdp = await page.context().newCDPSession(page)
  const [first, ...rest] = points
  const send = (type: string, touchPoints: { x: number; y: number }[]) =>
    cdp.send('Input.dispatchTouchEvent', { type: type as 'touchStart', touchPoints })
  if (pause > 0) {
    await send('touchStart', [first!])
    for (const point of rest) {
      await page.waitForTimeout(pause)
      await send('touchMove', [point])
    }
    await send('touchEnd', [])
  } else {
    await Promise.all([
      send('touchStart', [first!]),
      ...rest.map((point) => send('touchMove', [point])),
      send('touchEnd', []),
    ])
  }
  await cdp.detach()
}

// Upwards across the line, as a thumb scrolling the page down
for (const [gesture, pause] of [
  ['a slow drag', 40],
  ['a quick flick', 0],
] as const) {
  test(`${gesture} over the chart shows no tooltip`, async ({ page, core, ledger }) => {
    await seedHistory(page, core, ledger)
    await page.goto(chartsOf(ledger))
    const chart = page
      .locator('section', { has: balanceIn(page, 'EUR') })
      .locator('canvas')
      .first()
    await expect(chart).toBeVisible()

    const box = (await chart.boundingBox())!
    const x = box.x + box.width * 0.6
    const steps = Array.from({ length: 6 }, (_, step) => ({
      x,
      y: box.y + box.height * (0.8 - (0.6 * step) / 5),
    }))
    await touch(page, steps, pause)

    await page.waitForTimeout(300)
    await expect(tooltipFigure(page)).toHaveCount(0)
  })
}

test("a tap on the chart shows that bucket's balance", async ({ page, core, ledger }) => {
  await seedHistory(page, core, ledger)
  await page.goto(`${chartsOf(ledger)}?period=all`)
  const chart = page
    .locator('section', { has: balanceIn(page, 'EUR') })
    .locator('canvas')
    .first()
  await expect(chart).toBeVisible()

  const box = (await chart.boundingBox())!
  await touch(page, [{ x: box.x + box.width * 0.7, y: box.y + box.height * 0.5 }])

  await expect(tooltipFigure(page).first()).toBeVisible()
})
