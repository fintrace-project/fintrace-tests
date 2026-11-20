import type { Page } from '@playwright/test'
import { expect, localDaysAgo, test, type Core, type Ledger } from './fixtures.ts'

// Today a lunch and a salary, two days back a dinner, and a year and more back a bill, so last year is offered
async function seedDays(page: Page, core: Core, { workspace, account }: Ledger) {
  const operations = `/workspaces/${workspace.id}/operations`
  const at = (days: number) => localDaysAgo(page, days)
  await core.post(operations, {
    amount: 12.5,
    kind: 'EXPENSE',
    accountId: account.id,
    occurredAt: await at(0),
    comment: 'lunch',
  })
  await core.post(operations, { amount: 100, kind: 'INCOME', accountId: account.id, occurredAt: await at(0) })
  await core.post(operations, {
    amount: 30,
    kind: 'EXPENSE',
    accountId: account.id,
    occurredAt: await at(2),
    comment: 'dinner',
  })
  await core.post(operations, {
    amount: 5,
    kind: 'EXPENSE',
    accountId: account.id,
    occurredAt: await at(400),
  })
}

test("the heatmap shows the period's figures and the selected day's", async ({ page, core, ledger }) => {
  await seedDays(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}`)
  const card = page.getByRole('region', { name: 'Daily spending' })

  // Core's totals over the last twelve months: the lunch and the dinner, the bill being older
  await expect(card).toContainText('€42.50')
  await expect(card.getByRole('gridcell', { selected: true })).toHaveCount(1)
  // Each of the day's figures a label and its value, apart on a phone
  await expect(card).toContainText(/Spent\s*−€12\.50/)
  await expect(card).toContainText(/Income\s*\+€100\.00/)
  await expect(card).toContainText('2 transactions')
  // Core's expenseRatio: 42.50 of 100 income
  await expect(card).toContainText('43% of income')

  const modes = card.getByRole('group', { name: 'Show' })
  await modes.getByRole('button', { name: 'Transactions' }).click()
  await expect(card).toContainText('3 transactions')
  await modes.getByRole('button', { name: 'Net flow' }).click()
  await expect(card).toContainText('€57.50')
  // Core's netRatio, 0.575, as the share saved
  await expect(card).toContainText('58% of income saved')

  // The arrows walk the grid: up is the day before, sideways a week
  await card.getByRole('gridcell', { selected: true }).focus()
  await page.keyboard.press('ArrowUp')
  await page.keyboard.press('ArrowUp')
  await expect(card).toContainText(/Spent\s*−€30\.00/)
  await expect(card.getByRole('gridcell', { selected: true })).toBeFocused()
})

test('a year with operations is offered and shown whole', async ({ page, core, ledger }) => {
  await seedDays(page, core, ledger)
  const lastYear = await page.evaluate(() => new Date(Date.now() - 400 * 86_400_000).getFullYear())
  await page.goto(`/w/${ledger.workspace.id}`)
  const card = page.getByRole('region', { name: 'Daily spending' })

  // On a phone the periods are the row of chips above the grid
  const year = card
    .getByRole('group', { name: 'Period' })
    .getByRole('button', { name: String(lastYear) })
    .first()
  await year.click()
  await expect(year).toHaveAttribute('aria-pressed', 'true')
  await expect(card).toContainText(`1 Jan ${lastYear}`)
})

test("opening the feed on a day shows that day's operations alone", async ({ page, core, ledger }) => {
  await seedDays(page, core, ledger)
  await page.goto(`/w/${ledger.workspace.id}`)
  const card = page.getByRole('region', { name: 'Daily spending' })
  await expect(card).toContainText(/Spent\s*−€12\.50/)

  await card.getByRole('link', { name: 'Open in feed →' }).click()
  await expect(page).toHaveURL(/\/operations\?day=\d{4}-\d{2}-\d{2}$/)
  await expect(page.getByText(/^Income and expenses on /)).toBeVisible()
  await expect(page.getByText('lunch')).toBeVisible()
  await expect(page.getByText('dinner')).toHaveCount(0)

  await page.getByRole('button', { name: 'Show every day' }).click()
  await expect(page.getByText('dinner')).toBeVisible()
})
