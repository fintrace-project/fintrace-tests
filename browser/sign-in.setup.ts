import { expect, test as setup } from '@playwright/test'
import { credentials, SESSION } from './session.ts'

const BY_HAND_MS = 5 * 60_000

setup('a visitor signs in through the realm and lands on their workspaces', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/welcome$/)
  // Dismissed once here, the notice's cookie travels with the saved session
  await page.getByRole('button', { name: 'Got it' }).click()
  await page.getByRole('link', { name: 'Sign in', exact: true }).filter({ visible: true }).first().click()

  if (credentials) {
    // Keycloak's own form; its field ids are stable across its themes
    await page.locator('#username').fill(credentials.username)
    await page.locator('#password').fill(credentials.password)
    await page.locator('#kc-login').click()
  } else {
    setup.setTimeout(BY_HAND_MS + 30_000)
    console.log('Sign in as the test user in the browser window that opened; waiting up to 5 minutes')
  }

  await expect(page).toHaveURL(/\/workspaces$/, { timeout: credentials ? undefined : BY_HAND_MS })
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
  await page.context().storageState({ path: SESSION })
})
