import { expect, test } from './fixtures.ts'

test('an account is renamed, archived and restored, taking no entries while archived', async ({
  page,
  ledger,
}) => {
  const { workspace, account } = ledger
  await page.goto(`/w/${workspace.id}/accounts`)

  await page.getByRole('button', { name: 'Actions for Wallet' }).click()
  await page.getByRole('menuitem', { name: 'Rename' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Name', { exact: true }).fill('Purse')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()

  const menu = page.getByRole('button', { name: 'Actions for Purse' })
  await menu.click()
  await page.getByRole('menuitem', { name: 'Archive' }).click()
  await expect(page.getByRole('button', { name: 'Restore' })).toBeVisible()
  await expect(menu).toHaveCount(0)

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Purse' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Add operation' })).toHaveCount(0)

  await page.goto(`/w/${workspace.id}/accounts`)
  await page.getByRole('button', { name: 'Restore' }).click()
  await expect(menu).toBeVisible()
  await expect(page.getByRole('button', { name: 'Restore' })).toHaveCount(0)
})
