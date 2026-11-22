import { expect, test, type Category, type Workspace } from './fixtures.ts'

// The phone's Operations page: the add form on the page, the workspace's feed under it, the tab bar beneath both
test('an operation added on the Operations page clears the form and leads the feed', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, expenseRoot } = ledger
  const groceries = await core.post<Category>(`/workspaces/${workspace.id}/categories`, {
    name: 'Groceries',
    parentId: expenseRoot.id,
  })
  const comment = `added here ${Date.now()}`

  await page.goto(`/w/${workspace.id}/operations`)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Close' })).toHaveCount(0)
  await page.getByLabel('Amount').fill('7')
  await page.getByRole('button', { name: groceries.name }).click()
  await page.getByLabel('Comment (optional)').fill(comment)
  await page.getByRole('button', { name: 'Add expense' }).click()

  await expect(page.getByLabel('Amount')).toHaveValue('')
  await expect(page.getByLabel('Comment (optional)')).toHaveValue('')
  await expect(page.getByRole('region', { name: 'History' })).toContainText(comment)
  await expect(page).toHaveURL(new RegExp(`/w/${workspace.id}/operations$`))
})

test('the tab bar puts Operations in the middle', async ({ page, ledger }) => {
  await page.goto(`/w/${ledger.workspace.id}/operations`)
  const tabs = page.getByRole('navigation', { name: 'Workspace sections' }).getByRole('link')
  await expect(tabs).toHaveText(['Overview', 'Accounts', 'Operations', 'Statistics', 'Charts'])
  await expect(tabs.nth(2)).toHaveAttribute('aria-current', 'page')
})

test('an archived workspace shows its feed on the Operations page, with no form', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace } = ledger
  const current = await core.get<Workspace>(`/workspaces/${workspace.id}`)
  await core.post(`/workspaces/${workspace.id}/archive?version=${current.version}`, {})

  await page.goto(`/w/${workspace.id}/operations`)
  await expect(page.getByText('This workspace is archived.')).toBeVisible()
  await expect(page.getByRole('region', { name: 'History' })).toBeVisible()
  await expect(page.getByLabel('Amount')).toHaveCount(0)
})
