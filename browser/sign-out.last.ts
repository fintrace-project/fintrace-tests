import { expect, test } from '@playwright/test'

test('signing out ends the session and leads back to the welcome page', async ({ page }) => {
  await page.goto('/workspaces')
  await page.getByRole('button', { name: /^Signed in as/ }).click()
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/welcome$/)

  expect((await page.request.get('/api/v1/workspaces')).status()).toBe(401)
  await page.goto('/workspaces')
  await expect(page).toHaveURL(/\/welcome$/)
})
