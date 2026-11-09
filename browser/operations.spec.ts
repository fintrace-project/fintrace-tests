import {
  expect,
  localDaysAgo,
  NEGATIVE,
  POSITIVE,
  returnToTab,
  test,
  type Category,
  type OperationItem,
} from './fixtures.ts'

test('an expense is added back-dated and edited into an income, the balance following each', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account, expenseRoot } = ledger
  const groceries = await core.post<Category>(`/workspaces/${workspace.id}/categories`, {
    name: 'Groceries',
    parentId: expenseRoot.id,
  })
  const comment = `flow ${Date.now()}`
  const occurredAt = await localDaysAgo(page, 3)

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  const header = page.getByText(/^Balance /)
  await expect(header).toHaveText(POSITIVE('0.00'))

  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('25')
  await dialog.getByRole('button', { name: groceries.name }).click()
  await dialog.locator('input[type="datetime-local"]').fill(occurredAt)
  await dialog.getByLabel('Comment (optional)').fill(comment)
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(row).toContainText(groceries.name)
  await expect(row).toContainText('25.00')
  await expect(header).toHaveText(NEGATIVE('25.00'))
  // Back-dated as entered, never shifted to UTC
  const feed = await core.get<{ items: OperationItem[] }>(`/workspaces/${workspace.id}/feed`)
  expect(feed.items.find((item) => item.comment === comment)?.occurredAt).toMatch(
    new RegExp(`^${occurredAt}`),
  )

  await row.click()
  // The label is what a finger reaches: the radio inside it is visually hidden
  await dialog.locator('label', { hasText: 'Income' }).click()
  await dialog.getByLabel('Amount').fill('40')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expect(row).toContainText('40.00')
  await expect(header).toHaveText(POSITIVE('40.00'))
})

test('an operation whose category the page has not read yet shows a placeholder, then the name, never the id', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account, expenseRoot } = ledger
  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await expect(page.getByText('No operations yet.')).toBeVisible()

  // Made elsewhere, as another tab or device would
  const travel = await core.post<Category>(`/workspaces/${workspace.id}/categories`, {
    name: 'Travel',
    parentId: expenseRoot.id,
  })
  const comment = `elsewhere ${Date.now()}`
  await core.post(`/workspaces/${workspace.id}/operations`, {
    amount: 12,
    kind: 'EXPENSE',
    accountId: account.id,
    categoryId: travel.id,
    occurredAt: await localDaysAgo(page, 1),
    comment,
  })

  // The category list held back, so the feed's re-read lands first and the row cannot know the name yet
  let releaseCategories!: () => void
  const held = new Promise<void>((resolve) => (releaseCategories = resolve))
  await page.route('**/api/v1/workspaces/*/categories*', async (route) => {
    await held
    await route.continue()
  })

  await returnToTab(page)
  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(row).toContainText('…')
  await expect(row).not.toContainText(travel.id)

  releaseCategories()
  await expect(row).toContainText(travel.name)
  await expect(row).not.toContainText(travel.id)
})
