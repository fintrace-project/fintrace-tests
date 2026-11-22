import { expect, test, type Workspace } from './fixtures.ts'

// Entry is the app's own address, where sign-in returns: on a phone it opens the Operations page of the
// workspace last opened, its form ready
test('entry on a phone lands on the Operations page of the workspace last opened', async ({ page, ledger }) => {
  await page.goto(`/w/${ledger.workspace.id}`)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

  await page.goto('/')
  await expect(page).toHaveURL(new RegExp(`/w/${ledger.workspace.id}/operations$`))
  await expect(page.getByLabel('Amount')).toBeVisible()
  const tabs = page.getByRole('navigation', { name: 'Workspace sections' })
  await expect(tabs.getByRole('link', { name: 'Operations' })).toHaveAttribute('aria-current', 'page')
})

test('entry with no workspace opened lands on the list', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/workspaces$/)
})

test('entry after the last workspace was deleted lands on the list', async ({ page, core }) => {
  const workspace = await core.post<Workspace>('/workspaces', {
    workspaceName: `Browser flow ${Date.now()}`,
    defaultCurrency: 'EUR',
  })
  await page.goto(`/w/${workspace.id}`)
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await core.deleteWorkspaceIfThere(workspace.id)

  await page.goto('/')
  await expect(page).toHaveURL(/\/workspaces$/)
})

test.describe('on a desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 }, isMobile: false, hasTouch: false })

  test('entry lands on the list', async ({ page, ledger }) => {
    await page.goto(`/w/${ledger.workspace.id}`)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

    await page.goto('/')
    await expect(page).toHaveURL(/\/workspaces$/)
  })
})
