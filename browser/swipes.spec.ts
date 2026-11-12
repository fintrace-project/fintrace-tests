import { expect, localDaysAgo, NEGATIVE, POSITIVE, swipe, test, type Core, type Ledger } from './fixtures.ts'

function addExpense(core: Core, { workspace, account }: Ledger, comment: string, occurredAt: string) {
  return core.post(`/workspaces/${workspace.id}/operations`, {
    amount: 25,
    kind: 'EXPENSE',
    accountId: account.id,
    occurredAt,
    comment,
  })
}

test('swiping an operation left deletes it', async ({ page, core, ledger }) => {
  const comment = `swiped ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  await swipe(page, row, 'left')
  await expect(page.getByText('Operation deleted.')).toBeVisible()
  await expect(page.getByText('No operations yet.')).toBeVisible({ timeout: 10_000 })
  await expect(page.getByText(/^Balance /)).toHaveText(POSITIVE('0.00'))
})

test('a short quick flick left deletes an operation', async ({ page, core, ledger }) => {
  const comment = `flicked ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  // 60 px in three frames, about 1.2 px/ms: short of the threshold, a flick by its speed
  await swipe(page, page.getByRole('button', { name: new RegExp(comment) }), 'left', {
    distance: 60,
    steps: 3,
  })
  await expect(page.getByText('Operation deleted.')).toBeVisible()
  await expect(page.getByText('No operations yet.')).toBeVisible({ timeout: 10_000 })
})

test('the same short distance, slowly, springs back', async ({ page, core, ledger }) => {
  const comment = `held ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  // 60 px over half a second: short of the threshold and too slow for a flick
  await swipe(page, row, 'left', { distance: 60, stepPause: 50 })
  await expect(row).toBeInViewport()
  await expect(page.getByText('Operation deleted.')).toBeHidden()
})

test('a short quick flick right opens a new one made from it', async ({ page, core, ledger }) => {
  const comment = `flicked right ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  await swipe(page, page.getByRole('button', { name: new RegExp(comment) }), 'right', {
    distance: 60,
    steps: 3,
  })
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('Amount')).toHaveValue('25')
  await expect(dialog.getByLabel('Comment (optional)')).toHaveValue(comment)
})

test('undoing a swipe keeps the operation', async ({ page, core, ledger }) => {
  const comment = `kept ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  const row = page.getByRole('button', { name: new RegExp(comment) })
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  await swipe(page, row, 'left')
  await page.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Operation deleted.')).toBeHidden()

  await page.waitForTimeout(3_000)
  await page.reload()
  await expect(row).toBeVisible()
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))
})

test('swiping an operation right opens a new one made from it', async ({ page, core, ledger }) => {
  const comment = `repeated ${Date.now()}`
  await addExpense(core, ledger, comment, await localDaysAgo(page, 1))

  await page.goto(`/w/${ledger.workspace.id}/accounts/${ledger.account.id}`)
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('25.00'))

  await swipe(page, page.getByRole('button', { name: new RegExp(comment) }), 'right')
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('Amount')).toHaveValue('25')
  await expect(dialog.getByLabel('Comment (optional)')).toHaveValue(comment)
  await dialog.getByLabel('Amount').fill('30')
  await dialog.getByRole('button', { name: 'Add expense' }).click()

  await expect(dialog).toBeHidden()
  await expect(page.getByText(/^Balance /)).toHaveText(NEGATIVE('55.00'))
})
