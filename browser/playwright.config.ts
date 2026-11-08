import { defineConfig, devices } from '@playwright/test'
import { credentials, SESSION } from './session.ts'

// Flows through the running stack (§3.6), on the phone profile the web UI is laid out for first. Signing in
// is a project of its own, so every flow starts from the session it saved.
export default defineConfig({
  testDir: '.',
  workers: 1,
  fullyParallel: false,
  retries: 0,
  forbidOnly: !!process.env.CI,
  reporter: 'list',
  timeout: 60_000,
  use: {
    baseURL: process.env.BFF_URL ?? 'http://localhost:8100',
    ...devices['Pixel 7'],
    // The stack's pinned TZ, so "now" in the browser is never ahead of Core's
    timezoneId: 'Europe/Nicosia',
    locale: 'en-GB',
    trace: 'retain-on-failure',
  },
  projects: [
    // Visible when signing in by hand; the flows after it run headless
    // Placed and sized explicitly: on WSLg a window left to the defaults can land off screen, its icon alone
    // showing in the taskbar
    {
      name: 'sign-in',
      testMatch: 'sign-in.setup.ts',
      use: {
        headless: !!credentials,
        launchOptions: { args: ['--window-position=80,80', '--window-size=480,960'] },
      },
    },
    {
      name: 'flows',
      testMatch: '*.spec.ts',
      dependencies: ['sign-in'],
      use: { storageState: SESSION },
    },
    // Last, because it ends the session every flow shares
    {
      name: 'sign-out',
      testMatch: 'sign-out.last.ts',
      dependencies: ['flows'],
      use: { storageState: SESSION },
    },
  ],
})
