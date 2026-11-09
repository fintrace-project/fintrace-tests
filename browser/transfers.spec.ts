import type { Page } from '@playwright/test'
import {
  expect,
  localDaysAgo,
  NEGATIVE,
  POSITIVE,
  swipe,
  test,
  type Account,
  type Core,
  type Ledger,
} from './fixtures.ts'

function addAccount(core: Core, { workspace }: Ledger, name: string, currency = 'EUR') {
  return core.post<Account>(`/workspaces/${workspace.id}/accounts`, { name, currency })
}

async function expectBalance(page: Page, { workspace }: Ledger, id: string, figure: RegExp) {
  await page.goto(`/w/${workspace.id}/accounts/${id}`)
  await expect(page.getByText(/^Balance [^r]/)).toHaveText(figure)
}

async function seedTransfer(page: Page, core: Core, ledger: Ledger, target: Account, comment: string) {
  await core.post(`/workspaces/${ledger.workspace.id}/transfers`, {
    occurredAt: await localDaysAgo(page, 1),
    source: { accountId: ledger.account.id, amount: 30 },
    target: { accountId: target.id, amount: 30 },
    comment,
  })
}

test('a transfer is added, edited and turned round, both balances following', async ({
  page,
  core,
  ledger,
}) => {
  const savings = await addAccount(core, ledger, 'Savings')
  const comment = `moved ${Date.now()}`

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('label', { hasText: 'Transfer' }).click()
  await dialog.getByLabel('Amount', { exact: true }).fill('30')
  await dialog.getByLabel('Comment (optional)').fill(comment)
  await dialog.getByRole('button', { name: 'Add transfer' }).click()
  await expect(dialog).toBeHidden()

  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(row).toContainText('Wallet → Savings')
  await expectBalance(page, ledger, ledger.account.id, NEGATIVE('30.00'))
  await expectBalance(page, ledger, savings.id, POSITIVE('30.00'))

  await row.click()
  await expect(dialog.getByLabel('Amount', { exact: true })).toHaveValue('30')
  await expect(dialog.getByRole('radio', { name: 'Expense' })).toHaveCount(0)
  await dialog.getByLabel('Amount', { exact: true }).fill('45')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expectBalance(page, ledger, ledger.account.id, NEGATIVE('45.00'))

  await row.click()
  await dialog.getByRole('button', { name: 'Swap the accounts' }).click()
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()
  await expect(row).toContainText('Savings → Wallet')
  await expectBalance(page, ledger, ledger.account.id, POSITIVE('45.00'))
})

test('a transfer between currencies takes two amounts and shows the rate they imply', async ({
  page,
  core,
  ledger,
}) => {
  await addAccount(core, ledger, 'Savings')
  const dollars = await addAccount(core, ledger, 'Dollars', 'USD')

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.locator('label', { hasText: 'Transfer' }).click()
  await dialog.getByRole('button', { name: /^To\b/ }).click()
  await dialog.getByRole('button', { name: /Dollars/ }).click()

  await dialog.getByLabel('Amount sent').fill('100')
  await dialog.getByLabel('Amount received').fill('108.5')
  await expect(dialog.getByText(/^Rate 1\.085/)).toBeVisible()
  await dialog.getByRole('button', { name: 'Add transfer' }).click()
  await expect(dialog).toBeHidden()

  await expect(page.getByText(/^Rate 1\.085/)).toBeVisible()
  await expectBalance(page, ledger, ledger.account.id, NEGATIVE('100.00'))
  await expectBalance(page, ledger, dollars.id, POSITIVE('108.50'))
})

test('a transfer needs a second account, and what was typed survives the switch', async ({
  page,
  ledger,
}) => {
  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await page.getByRole('button', { name: 'Add operation' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Amount').fill('12')

  await dialog.locator('label', { hasText: 'Transfer' }).click()
  await expect(dialog.getByText('fewer than two open')).toBeVisible()

  await dialog.locator('label', { hasText: 'Expense' }).click()
  await expect(dialog.getByLabel('Amount')).toHaveValue('12')
})

test('swiping a transfer left deletes both legs', async ({ page, core, ledger }) => {
  const savings = await addAccount(core, ledger, 'Savings')
  const comment = `swiped ${Date.now()}`
  await seedTransfer(page, core, ledger, savings, comment)

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(/^Balance [^r]/)).toHaveText(NEGATIVE('30.00'))
  await swipe(page, page.getByRole('button', { name: new RegExp(comment) }), 'left')
  await expect(page.getByText('Transfer deleted.')).toBeVisible()
  await expect(page.getByText('No operations yet.')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByText(/^Balance [^r]/)).toHaveText(POSITIVE('0.00'))
  await expectBalance(page, ledger, savings.id, POSITIVE('0.00'))
})

test('swiping a transfer right opens a new one made from it', async ({ page, core, ledger }) => {
  const savings = await addAccount(core, ledger, 'Savings')
  const comment = `repeated ${Date.now()}`
  await seedTransfer(page, core, ledger, savings, comment)

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(/^Balance [^r]/)).toHaveText(NEGATIVE('30.00'))
  await swipe(page, page.getByRole('button', { name: new RegExp(comment) }), 'right')

  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('Amount', { exact: true })).toHaveValue('30')
  await expect(dialog.getByLabel('Comment (optional)')).toHaveValue(comment)
  await dialog.getByLabel('Amount', { exact: true }).fill('20')
  await dialog.getByRole('button', { name: 'Add transfer' }).click()
  await expect(dialog).toBeHidden()
  await expect(page.getByText(/^Balance [^r]/)).toHaveText(NEGATIVE('50.00'))
})

test('a transfer touching an archived account offers no edit', async ({ page, core, ledger }) => {
  const savings = await addAccount(core, ledger, 'Savings')
  const comment = `frozen ${Date.now()}`
  await seedTransfer(page, core, ledger, savings, comment)
  await core.delete(`/workspaces/${ledger.workspace.id}/accounts/${savings.id}`)

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(comment)).toBeVisible()
  await expect(page.getByRole('button', { name: new RegExp(comment) })).toHaveCount(0)
})
