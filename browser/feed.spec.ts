import { expect, localDaysAgo, NEGATIVE, POSITIVE, test, type Account } from './fixtures.ts'

test('a transfer is one row in both accounts and moves both balances', async ({ page, core, ledger }) => {
  const { workspace, account } = ledger
  const savings = await core.post<Account>(`/workspaces/${workspace.id}/accounts`, {
    name: 'Savings',
    currency: 'EUR',
  })
  const comment = `moved ${Date.now()}`
  await core.post(`/workspaces/${workspace.id}/transfers`, {
    occurredAt: await localDaysAgo(page, 1),
    source: { accountId: account.id, amount: 30 },
    target: { accountId: savings.id, amount: 30 },
    comment,
  })

  for (const [id, figure] of [
    [account.id, NEGATIVE('30.00')],
    [savings.id, POSITIVE('30.00')],
  ] as const) {
    await page.goto(`/w/${workspace.id}/accounts/${id}`)
    await expect(page.getByText(/^Balance /)).toHaveText(figure)
    await expect(page.getByText('Wallet → Savings')).toHaveCount(1)
  }
})

test('an anchor that disagrees with the operations shows what went unrecorded', async ({
  page,
  core,
  ledger,
}) => {
  const { workspace, account } = ledger
  const anchors = `/workspaces/${workspace.id}/accounts/${account.id}/balance-anchors`
  await core.post(anchors, { value: 100 })
  await core.post(anchors, { value: 80 })

  await page.goto(`/w/${workspace.id}/accounts/${account.id}`)
  await expect(page.getByText(POSITIVE('80.00'))).toBeVisible()
  await expect(page.getByText('Balance recorded')).toHaveCount(2)
  await expect(page.getByText(/20\.00 unrecorded$/)).toHaveCount(1)
})
