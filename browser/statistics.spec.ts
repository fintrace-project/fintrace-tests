import type { Page } from '@playwright/test'
import { expect, localDaysAgo, test, type Category, type Core, type Ledger } from './fixtures.ts'

async function category(core: Core, { workspace }: Ledger, name: string, parentId: string) {
  return core.post<Category>(`/workspaces/${workspace.id}/categories`, { name, parentId })
}

async function expense(
  page: Page,
  core: Core,
  { workspace, account }: Ledger,
  amount: number,
  categoryId: string,
  days = 0,
) {
  await core.post(`/workspaces/${workspace.id}/operations`, {
    amount,
    kind: 'EXPENSE',
    accountId: account.id,
    categoryId,
    occurredAt: await localDaysAgo(page, days),
  })
}

// The panel holding a branch's breakdown, by its heading
const branch = (page: Page, name: 'Expenses' | 'Income') =>
  page.locator('section', { has: page.getByRole('heading', { name, exact: true }) })

test('the breakdown drills into a category, shows its own money, and Back goes up a level', async ({
  page,
  core,
  ledger,
}) => {
  const food = await category(core, ledger, 'Food', ledger.expenseRoot.id)
  const groceries = await category(core, ledger, 'Groceries', food.id)
  const rent = await category(core, ledger, 'Rent', ledger.expenseRoot.id)
  await expense(page, core, ledger, 50, rent.id)
  await expense(page, core, ledger, 30, groceries.id)
  await expense(page, core, ledger, 10, food.id)
  await core.post(`/workspaces/${ledger.workspace.id}/operations`, {
    amount: 100,
    kind: 'INCOME',
    accountId: ledger.account.id,
    occurredAt: await localDaysAgo(page, 0),
  })

  await page.goto(`/w/${ledger.workspace.id}/statistics`)
  const expenses = branch(page, 'Expenses')
  const rows = expenses.getByRole('list', { name: 'Expenses by category' }).getByRole('listitem')
  // Core's rollup: Food carries its groceries and its own ten, Rent leads
  await expect(expenses).toContainText('€90.00')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toHaveText(/Rent.*€50\.00/)
  await expect(rows.nth(1)).toHaveText(/Food.*€40\.00/)

  await rows.nth(1).getByRole('button').click()
  await expect(page).toHaveURL(new RegExp(`expense=${food.id}`))
  await expect(expenses.getByRole('navigation', { name: 'Category path' })).toContainText('All expenses')
  await expect(rows).toHaveCount(2)
  await expect(rows.nth(0)).toHaveText(/Groceries.*€30\.00/)
  // The parent's selfTotal, a slice of its own (§11.2)
  await expect(rows.nth(1)).toHaveText(/Food itself.*€10\.00/)

  await page.goBack()
  await expect(rows.nth(0)).toHaveText(/Rent.*€50\.00/)

  // On a phone one branch at a time, behind the Categories page's switch
  // The label is what a finger reaches, the radio inside it visually hidden; the radio follows the URL a render
  // later, so its state is awaited rather than checked on the spot
  await page.locator('label', { hasText: 'Income' }).click()
  await expect(page.getByRole('radio', { name: 'Income' })).toBeChecked()
  await expect(page).toHaveURL(/kind=INCOME/)
  const income = branch(page, 'Income')
  await expect(income.getByRole('list', { name: 'Income by category' })).toContainText(/Others.*€100\.00/)
  await expect(expenses).toBeHidden()
})

test('the month dropdown picks all of a past year', async ({ page, core, ledger }) => {
  const rent = await category(core, ledger, 'Rent', ledger.expenseRoot.id)
  await expense(page, core, ledger, 5, rent.id, 400)
  const lastYear = await page.evaluate(() => new Date(Date.now() - 400 * 86_400_000).getFullYear())

  await page.goto(`/w/${ledger.workspace.id}/statistics`)
  const expenses = branch(page, 'Expenses')
  await expect(expenses).toContainText('No expenses in this period')

  await page.getByRole('button', { name: /pick another month/ }).click()
  const dropdown = page.getByRole('dialog')
  for (let i = 0; i < 3 && !(await dropdown.getByText(`All of ${lastYear}`).isVisible()); i++) {
    await dropdown.getByRole('button', { name: 'Previous year' }).click()
  }
  await dropdown.getByRole('button', { name: `All of ${lastYear}` }).click()

  await expect(page).toHaveURL(new RegExp(`period=${lastYear}`))
  await expect(
    page.getByRole('button', { name: new RegExp(`^${lastYear}, pick another month`) }),
  ).toBeVisible()
  await expect(expenses.getByRole('list', { name: 'Expenses by category' })).toContainText(/Rent.*€5\.00/)
})

test("the overview's panel shows the five largest, the rest as Other, over the span picked", async ({
  page,
  core,
  ledger,
}) => {
  const names = ['Rent', 'Food', 'Transport', 'Cafes', 'Books', 'Gifts']
  for (const [at, name] of names.entries()) {
    const each = await category(core, ledger, name, ledger.expenseRoot.id)
    await expense(page, core, ledger, 60 - at * 10, each.id)
  }
  // Older than every span but All, and larger than all of them
  const old = await category(core, ledger, 'Holiday', ledger.expenseRoot.id)
  await expense(page, core, ledger, 500, old.id, 400)

  await page.goto(`/w/${ledger.workspace.id}`)
  const panel = page.locator('section', { has: page.getByRole('heading', { name: 'Expense breakdown' }) })
  const rows = panel.getByRole('list', { name: 'Expenses by category' }).getByRole('listitem')
  // 1M first: Gifts' ten is Core's "other"
  await expect(rows).toHaveCount(6)
  await expect(rows.nth(0)).toHaveText(/Rent.*€60\.00/)
  await expect(rows.nth(5)).toHaveText(/Other.*€10\.00/)

  await panel.getByRole('group', { name: 'Range' }).getByRole('button', { name: 'All' }).click()
  await expect(rows.nth(0)).toHaveText(/Holiday.*€500\.00/)
  await expect(rows.nth(5)).toHaveText(/Other.*€30\.00/)
})
