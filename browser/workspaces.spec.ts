import { expect, test, type Workspace } from './fixtures.ts'

test('a workspace goes from new through archived to deleted, its first account closing import', async ({
  page,
  core,
}) => {
  const name = `Browser flow ${Date.now()}`
  await page.goto('/workspaces/new')
  await page.getByLabel('Name', { exact: true }).fill(name)
  await page.getByRole('button', { name: 'Create workspace' }).click()
  await expect(page).toHaveURL(/\/w\/[0-9a-f-]{36}$/)
  const id = new URL(page.url()).pathname.split('/').pop()!

  try {
    const banner = page.getByText('Anything you add yourself closes import for good.')
    await expect(banner).toBeVisible()

    await page.goto(`/w/${id}/accounts`)
    await page.getByRole('button', { name: 'Add account' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog.getByText('Adding one closes import for this workspace for good.')).toBeVisible()
    await dialog.getByLabel('Name', { exact: true }).fill('Current')
    await dialog.getByLabel('Balance today (optional)').fill('100')
    await dialog.getByRole('button', { name: 'Add account' }).click()
    await expect(dialog).toBeHidden()
    await expect(page.getByText('€100.00').first()).toBeVisible()
    await expect(banner).toBeHidden()

    await page.goto(`/w/${id}/import`)
    await expect(page.getByText('Import is closed')).toBeVisible()

    await page.goto(`/w/${id}`)
    await page.getByRole('button', { name: 'Archive', exact: true }).click()
    const archived = page.getByText('This workspace is archived.')
    await expect(archived).toBeVisible()
    await page.goto(`/w/${id}/operations`)
    await expect(archived).toBeVisible()
    await expect(page.getByRole('button', { name: 'Add operation' })).toHaveCount(0)

    await page.getByRole('button', { name: 'Unarchive' }).click()
    await expect(archived).toBeHidden()
    await expect(page.getByRole('button', { name: 'Add operation' })).toBeVisible()

    const renamed = `${name} b`
    await page.goto(`/w/${id}/settings`)
    await page.getByLabel('Name', { exact: true }).fill(renamed)
    await page.getByRole('button', { name: 'Save' }).click()
    await expect(page.getByText('Saved.')).toBeVisible()

    await page.getByRole('button', { name: 'Delete workspace…' }).click()
    const confirm = page.getByRole('dialog')
    const forever = confirm.getByRole('button', { name: 'Delete forever' })
    await expect(forever).toBeDisabled()
    await confirm.getByRole('textbox').fill(renamed)
    await forever.click()
    await expect(page).toHaveURL(/\/workspaces$/)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    await expect(page.getByText(renamed)).toHaveCount(0)
  } finally {
    await core.deleteWorkspaceIfThere(id)
  }
})

test('a save over a workspace changed elsewhere is refused and offers a reload', async ({
  page,
  core,
  ledger,
}) => {
  const { id } = ledger.workspace
  await page.goto(`/w/${id}/settings`)
  const name = page.getByLabel('Name', { exact: true })
  await expect(name).toBeVisible()

  const current = await core.get<Workspace>(`/workspaces/${id}`)
  await core.put(`/workspaces/${id}`, {
    version: current.version,
    workspaceName: 'Renamed elsewhere',
    defaultCurrency: 'EUR',
  })

  await name.fill('Renamed here')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('This changed since the page loaded.')).toBeVisible()

  await page.getByRole('button', { name: 'Reload' }).click()
  await expect(name).toHaveValue('Renamed elsewhere')
})

test('an operation submitted after the workspace was archived elsewhere is refused, the form kept', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('25')

  const current = await core.get<Workspace>(`/workspaces/${workspace.id}`)
  await core.post(`/workspaces/${workspace.id}/archive?version=${current.version}`, undefined)

  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toContainText('not allowed to write')
  await expect(dialog.getByLabel('Amount')).toHaveValue('25')
})

test('a workspace that does not exist shows not found', async ({ page }) => {
  await page.goto('/w/00000000-0000-7000-8000-000000000000')
  await expect(page.getByText('Page not found')).toBeVisible()
})
