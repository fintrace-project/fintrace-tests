import { expect, longPress, test, type Category, type Core, type Ledger } from './fixtures.ts'
import type { Locator, Page } from '@playwright/test'

function addCategory({ workspace }: Ledger, core: Core, name: string, parentId: string) {
  return core.post<Category>(`/workspaces/${workspace.id}/categories`, { name, parentId })
}

const openPage = (page: Page, { workspace }: Ledger) => page.goto(`/w/${workspace.id}/categories`)
const expenses = (page: Page) => page.getByRole('region', { name: 'Expense' })
// Anchored, so "Actions for Food" is not the tile; a parent's badge is read before its name
const tile = (within: Locator, name: string) =>
  within.getByRole('button', { name: new RegExp(`^(\\d+ subcategor\\w+ )?${name}$`) })

async function chooseFromMenu(page: Page, target: Locator, item: string) {
  await longPress(page, target)
  await page.getByRole('menuitem', { name: item }).click()
}

test('a category and a subcategory are added and renamed', async ({ page, ledger }) => {
  await openPage(page, ledger)
  const dialog = page.getByRole('dialog')

  await expenses(page).getByRole('button', { name: 'New category' }).click()
  await dialog.getByLabel('Name').fill('Food')
  await dialog.getByRole('button', { name: 'Add' }).click()
  await expect(dialog).toBeHidden()

  // A hold opens the menu and nothing else: the tile stays closed
  await longPress(page, tile(expenses(page), 'Food'))
  await expect(page.getByRole('menu')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toBeHidden()
  await expect(tile(expenses(page), 'Food')).toHaveAttribute('aria-pressed', 'false')
  await expect(page).not.toHaveURL(/category=/)

  await tile(expenses(page), 'Food').click()
  await expenses(page).getByRole('button', { name: 'New subcategory' }).click()
  await dialog.getByLabel('Name').fill('Cafes')
  await dialog.getByRole('button', { name: 'Add' }).click()
  await expect(dialog).toBeHidden()
  await expect(expenses(page).getByRole('button', { name: '1 subcategory Food' })).toBeVisible()

  await chooseFromMenu(page, tile(expenses(page), 'Cafes'), 'Rename')
  await dialog.getByLabel('Name').fill('Coffee')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expect(tile(expenses(page), 'Coffee')).toBeVisible()
  await expect(tile(expenses(page), 'Cafes')).toHaveCount(0)
})

test('a move offers no place inside the category itself, and takes its subtree along', async ({
  page,
  core,
  ledger,
}) => {
  const food = await addCategory(ledger, core, 'Food', ledger.expenseRoot.id)
  const cafes = await addCategory(ledger, core, 'Cafes', food.id)
  await addCategory(ledger, core, 'Bars', cafes.id)
  await openPage(page, ledger)

  await tile(expenses(page), 'Food').click()
  await chooseFromMenu(page, tile(expenses(page), 'Cafes'), 'Move')
  const dialog = page.getByRole('dialog')
  // Opens where the category is now, its own tile no target among Food's children
  await expect(tile(dialog, 'Food')).toHaveAttribute('aria-pressed', 'true')
  await expect(tile(dialog, 'Cafes')).toHaveCount(0)
  await dialog.getByLabel('Search categories').fill('Bars')
  await expect(dialog.getByText('No category found.')).toBeVisible()
  await dialog.getByLabel('Search categories').fill('')

  await dialog.getByRole('button', { name: 'Top level' }).click()
  await dialog.getByRole('button', { name: 'Move', exact: true }).click()
  await expect(dialog).toBeHidden()

  const moved = await core.get<Category & { parentId: string }>(
    `/workspaces/${ledger.workspace.id}/categories/${cafes.id}`,
  )
  expect(moved.parentId).toBe(ledger.expenseRoot.id)
  await expect(expenses(page).getByRole('button', { name: '1 subcategory Cafes' })).toBeVisible()
})

test('a move reaches a target below the top level through the panel under its row', async ({
  page,
  core,
  ledger,
}) => {
  const food = await addCategory(ledger, core, 'Food', ledger.expenseRoot.id)
  const cafes = await addCategory(ledger, core, 'Cafes', food.id)
  const transport = await addCategory(ledger, core, 'Transport', ledger.expenseRoot.id)
  const taxi = await addCategory(ledger, core, 'Taxi', transport.id)
  await openPage(page, ledger)

  await tile(expenses(page), 'Food').click()
  await chooseFromMenu(page, tile(expenses(page), 'Cafes'), 'Move')
  const dialog = page.getByRole('dialog')
  await tile(dialog, 'Transport').click()
  await tile(dialog, 'Taxi').click()
  await expect(tile(dialog, 'Taxi')).toHaveAttribute('aria-pressed', 'true')
  await dialog.getByRole('button', { name: 'Move', exact: true }).click()
  await expect(dialog).toBeHidden()

  const moved = await core.get<Category & { parentId: string }>(
    `/workspaces/${ledger.workspace.id}/categories/${cafes.id}`,
  )
  expect(moved.parentId).toBe(taxi.id)
})

test('archiving a category takes its subcategories out of the page and the picker', async ({
  page,
  core,
  ledger,
}) => {
  const food = await addCategory(ledger, core, 'Food', ledger.expenseRoot.id)
  await addCategory(ledger, core, 'Cafes', food.id)
  await openPage(page, ledger)

  await chooseFromMenu(page, tile(expenses(page), 'Food'), 'Archive')
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('cannot be undone')
  await dialog.getByRole('button', { name: 'Archive' }).click()
  await expect(dialog).toBeHidden()
  await expect(tile(expenses(page), 'Food')).toHaveCount(0)

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const form = page.getByRole('dialog')
  await form.getByLabel('Search categories').fill('Cafes')
  await expect(form.getByText('No category found.')).toBeVisible()
})

test('a system category offers no actions and says why', async ({ page, ledger }) => {
  await openPage(page, ledger)
  const others = tile(expenses(page), 'Others')

  await longPress(page, others)
  await expect(page.getByRole('menu')).toHaveCount(0)

  await others.click()
  await expect(expenses(page).getByText('It cannot be renamed, moved or archived.')).toBeVisible()
})

test.describe('on a desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 }, isMobile: false, hasTouch: false })

  test('both branches are side by side, and a category has a menu button of its own', async ({
    page,
    core,
    ledger,
  }) => {
    await addCategory(ledger, core, 'Food', ledger.expenseRoot.id)
    await openPage(page, ledger)

    await expect(expenses(page)).toBeVisible()
    await expect(page.getByRole('region', { name: 'Income' })).toBeVisible()
    // Nothing to switch with both on screen
    await expect(page.getByRole('radio', { name: 'Income' })).toBeHidden()

    await expenses(page).getByRole('button', { name: 'Actions for Food' }).click()
    await page.getByRole('menuitem', { name: 'Rename' }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByLabel('Name').fill('Groceries')
    await dialog.getByRole('button', { name: 'Save' }).click()
    await expect(dialog).toBeHidden()
    await expect(tile(expenses(page), 'Groceries')).toBeVisible()
  })
})
