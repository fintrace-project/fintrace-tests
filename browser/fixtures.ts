import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test'

// Only the fields the flows read; the web UI's generated client is the contract, this is a test's view of it
export interface Workspace {
  id: string
  version: number
}
export interface Account {
  id: string
  name: string
}
export interface Category {
  id: string
  name: string
  systemCode?: string | null
}
export interface OperationItem {
  type: 'OPERATION'
  id: string
  comment?: string | null
  occurredAt: string
}

// Core through the BFF, on the signed-in browser's session cookie. The BFF refuses an unsafe method without
// an Origin naming it, as a browser would send.
export class Core {
  private readonly request: APIRequestContext
  private readonly origin: string

  constructor(request: APIRequestContext, origin: string) {
    this.request = request
    this.origin = origin
  }

  get<T>(path: string): Promise<T> {
    return this.call('GET', path)
  }

  post<T>(path: string, data: unknown): Promise<T> {
    return this.call('POST', path, data)
  }

  delete(path: string): Promise<void> {
    return this.call('DELETE', path)
  }

  private async call<T>(method: string, path: string, data?: unknown): Promise<T> {
    const response = await this.request.fetch(`/api/v1${path}`, {
      method,
      data,
      headers: { Origin: this.origin },
    })
    expect(response.ok(), `${method} ${path} answered ${response.status()}: ${await response.text()}`).toBe(
      true,
    )
    return (response.status() === 204 ? undefined : await response.json()) as T
  }
}

export interface Ledger {
  workspace: Workspace
  account: Account
  expenseRoot: Category
}

// Every flow gets a workspace of its own with one account and no balance: an opening balance would be an
// anchor dated now, and a back-dated operation before it would not move the balance (§4.6)
export const test = base.extend<{ core: Core; ledger: Ledger }>({
  core: async ({ page, baseURL }, use) => {
    await use(new Core(page.request, new URL(baseURL!).origin))
  },
  ledger: async ({ core }, use) => {
    const workspace = await core.post<Workspace>('/workspaces', {
      workspaceName: `Browser flow ${Date.now()}`,
      defaultCurrency: 'EUR',
    })
    try {
      const account = await core.post<Account>(`/workspaces/${workspace.id}/accounts`, {
        name: 'Wallet',
        currency: 'EUR',
      })
      const categories = await core.get<{ items: Category[] }>(`/workspaces/${workspace.id}/categories`)
      const expenseRoot = categories.items.find((category) => category.systemCode === 'EXPENSE_ROOT')!
      await use({ workspace, account, expenseRoot })
    } finally {
      // Deleted even when the flow failed; the version has moved with the first write
      const current = await core.get<Workspace>(`/workspaces/${workspace.id}`)
      await core.delete(`/workspaces/${workspace.id}?version=${current.version}`)
    }
  },
})

export { expect }

// A floating `YYYY-MM-DDTHH:mm` in the browser's zone, `days` before now
export function localDaysAgo(page: Page, days: number): Promise<string> {
  return page.evaluate((days) => {
    const date = new Date(Date.now() - days * 86_400_000)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
  }, days)
}

// What a return to the tab does: TanStack Query re-reads what is stale on visibilitychange (§10.5)
export function returnToTab(page: Page): Promise<void> {
  return page.evaluate(() => {
    document.dispatchEvent(new Event('visibilitychange', { bubbles: true }))
  })
}
