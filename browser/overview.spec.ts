import { expect, test, type Category } from './fixtures.ts'

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
