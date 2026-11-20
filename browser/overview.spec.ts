import { expect, localDaysAgo, test, type Category } from './fixtures.ts'

test('the overview totals the balances per currency, and an expense moves its currency only', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account, expenseRoot } = ledger
  const accounts = `/workspaces/${workspace.id}/accounts`
  await core.post(accounts, { name: 'Savings', currency: 'EUR', initialBalance: 40 })
  await core.post(accounts, { name: 'Dollars', currency: 'USD', initialBalance: 25 })
  const groceries = await core.post<Category>(`/workspaces/${workspace.id}/categories`, {
    name: 'Groceries',
    parentId: expenseRoot.id,
  })

  await page.goto(`/w/${workspace.id}`)
  const widget = page.getByRole('region', { name: 'Total balance' })
  const headline = widget.locator('p').first()
  const rows = widget.getByRole('button')
  // Wallet's 0 and Savings' 40 summed by Core, one row per currency in Core's order
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toContainText('€40.00')
  await expect(rows.nth(1)).toContainText('US$25.00')

  // The workspace's own currency is the display one until a row is picked; the headline is its own total, and
  // the balance chart beside it follows the pick
  const panelFigure = page
    .locator('section', { has: page.getByRole('heading', { name: 'Balance', exact: true }) })
    .locator('p')
    .first()
  await expect(rows.nth(0)).toHaveAttribute('aria-pressed', 'true')
  await expect(headline).toHaveText('€40.00')
  await expect(panelFigure).toHaveText('€40.00')
  await rows.nth(1).click()
  await expect(rows.nth(1)).toHaveAttribute('aria-pressed', 'true')
  await expect(headline).toHaveText('US$25.00')
  await expect(panelFigure).toHaveText('US$25.00')
  await expect(rows.nth(0)).toHaveAttribute('aria-pressed', 'false')

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('25')
  await dialog.getByRole('button', { name: groceries.name }).click()
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  // Back by the app's own link, not a reload: the write must have refreshed the cached totals
  await page.getByRole('link', { name: 'Overview' }).first().click()
  await expect(rows.nth(0)).toContainText('€15.00')
  await expect(rows.nth(1)).toContainText('US$25.00')
})

test("this month's card and the expenses panel show the month, in the display currency", async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  const operations = `/workspaces/${workspace.id}/operations`
  await core.post(`/workspaces/${workspace.id}/accounts`, {
    name: 'Dollars',
    currency: 'USD',
    initialBalance: 25,
  })
  const now = await localDaysAgo(page, 0)
  await core.post(operations, { amount: 100, kind: 'INCOME', accountId: account.id, occurredAt: now })
  await core.post(operations, { amount: 12.5, kind: 'EXPENSE', accountId: account.id, occurredAt: now })

  await page.goto(`/w/${workspace.id}`)
  // Core's figures for this month's bucket, signed by their side as the feeds sign an operation
  const card = page.getByRole('region', { name: 'This month' })
  await expect(card).toContainText('+€100.00')
  await expect(card).toContainText('−€12.50')

  const panel = page.locator('section', { has: page.getByRole('heading', { name: 'Expenses by month' }) })
  await expect(panel.locator('canvas')).toHaveCount(1)

  // The card follows the currency the balance widget picks; the dollars had no cashflow this month
  await page.getByRole('region', { name: 'Total balance' }).getByRole('button').nth(1).click()
  await expect(card).toContainText('+US$0.00')
  await expect(card).toContainText('−US$0.00')
  await expect(panel).toContainText('No expenses over the past year.')
})

test("this month's card ranks its top four categories, the rest as Other, and a write re-ranks them", async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account, expenseRoot } = ledger
  const now = await localDaysAgo(page, 0)
  const spend = [
    ['Rent', 50],
    ['Groceries', 40],
    ['Transport', 30],
    ['Cafes', 20],
    ['Books', 10],
  ] as const
  for (const [name, amount] of spend) {
    const category = await core.post<Category>(`/workspaces/${workspace.id}/categories`, {
      name,
      parentId: expenseRoot.id,
    })
    await core.post(`/workspaces/${workspace.id}/operations`, {
      amount,
      kind: 'EXPENSE',
      accountId: account.id,
      categoryId: category.id,
      occurredAt: now,
    })
  }

  await page.goto(`/w/${workspace.id}`)
  const card = page.getByRole('region', { name: 'This month' })
  const rows = card.getByRole('list', { name: 'Expenses by category' }).getByRole('listitem')
  // Core's four largest, then Books alone in Other
  await expect(rows).toHaveCount(5)
  await expect(rows.nth(0)).toHaveText(/Rent.*€50\.00/)
  await expect(rows.nth(3)).toHaveText(/Cafes.*€20\.00/)
  await expect(rows.nth(4)).toHaveText(/Other.*€10\.00/)
  await expect(card).toContainText('No income this month')

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('45')
  await dialog.getByRole('button', { name: 'Books' }).click()
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  // Back by the app's own link: the write refreshed the cached ranking, Books first and Cafes now Other
  await page.getByRole('link', { name: 'Overview' }).first().click()
  await expect(rows.nth(0)).toHaveText(/Books.*€55\.00/)
  await expect(rows.nth(4)).toHaveText(/Other.*€20\.00/)
})
