import { expect, POSITIVE, test, type OperationItem } from './fixtures.ts'

test('a balance is recorded from the account page, and deleting it brings back the one before', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  await core.post(`/workspaces/${workspace.id}/accounts/${account.id}/balance-anchors`, { value: 100 })

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  // Not the feed's "Balance recorded" rows
  const header = page.getByText(/^Balance (?!recorded)/)
  await expect(header).toHaveText(POSITIVE('100.00'))

  await page.getByRole('button', { name: 'Record balance' }).click()
  const dialog = page.getByRole('dialog')
  // Opens on the figure Core computes, to be replaced by the one counted
  const value = dialog.getByLabel('Balance now, EUR')
  await expect(value).toHaveValue('100')
  await value.fill('80')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect(dialog).toBeHidden()

  await expect(header).toHaveText(POSITIVE('80.00'))
  await expect(page.getByText('Balance recorded')).toHaveCount(2)
  await expect(page.getByRole('button', { name: /^Not accounted for/ })).toContainText('20.00')

  // Only the latest may be deleted (§10.4), so only its row offers it
  const remove = page.getByRole('button', { name: 'Delete the latest recorded balance' })
  await expect(remove).toHaveCount(1)
  await remove.click()
  await expect(dialog.getByText('Delete recorded balance?')).toBeVisible()
  // What the deletion leaves: the current balance less the difference the anchor absorbed
  await expect(dialog.locator('dd').last()).toHaveText(/80\.00\D*→\D*100\.00/)
  await dialog.getByRole('button', { name: 'Delete' }).click()
  await expect(dialog).toBeHidden()

  await expect(header).toHaveText(POSITIVE('100.00'))
  await expect(page.getByText('Balance recorded')).toHaveCount(1)
  await expect(page.getByText('Not accounted for')).toHaveCount(0)
})

test('a difference is booked as an operation just before its anchor, which leaves the balance as it was', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  const anchors = `/workspaces/${workspace.id}/accounts/${account.id}/balance-anchors`
  await core.post(anchors, { value: 100 })
  // Within the same minute as the first: the operation's time has to fit between the two
  const counted = await core.post<{ occurredAt: string }>(anchors, { value: 80 })

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  const header = page.getByText(/^Balance (?!recorded)/)
  await expect(header).toHaveText(POSITIVE('80.00'))

  await page.getByRole('button', { name: /^Not accounted for/ }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByLabel('Amount')).toHaveValue('20')
  await dialog.getByRole('button', { name: 'Add expense' }).click()
  await expect(dialog).toBeHidden()

  // Explained now, so nothing is left unaccounted for, and the anchor still says what was counted
  await expect(page.getByText('Not accounted for')).toHaveCount(0)
  await expect(header).toHaveText(POSITIVE('80.00'))
  const feed = await core.get<{ items: OperationItem[] }>(`/workspaces/${workspace.id}/feed`)
  const booked = feed.items.find((item) => item.type === 'OPERATION')!
  expect(booked.occurredAt < counted.occurredAt).toBe(true)
})

test('the arrows step through every recorded balance, one with no difference included', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  const anchors = `/workspaces/${workspace.id}/accounts/${account.id}/balance-anchors`
  await core.post(anchors, { value: 100 })
  // The same figure again: no difference, so no DIFFERENCE item for the arrows to have found before
  await core.post(anchors, { value: 100 })

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  const arrows = page.getByRole('group', { name: 'Recorded balances' })
  const older = arrows.getByRole('link', { name: 'Older recorded balance' })
  const marked = page.locator('li[aria-current="true"]')

  await older.click()
  await expect(page).toHaveURL(/[?&]at=/)
  await expect(marked).toContainText('Balance recorded')
  await expect(page.getByText('Not accounted for')).toHaveCount(0)
  // The newest anchor, so the one its row offers to delete
  await expect(marked.getByRole('button', { name: 'Delete the latest recorded balance' })).toBeVisible()

  await older.click()
  await expect(marked).toContainText('Balance recorded')
  await expect(marked.getByRole('button', { name: 'Delete the latest recorded balance' })).toHaveCount(0)
  // Nothing older than the account's first anchor
  await expect(arrows.getByRole('button', { name: 'Older recorded balance' })).toBeDisabled()
})
