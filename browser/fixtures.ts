import { test as base, expect, type APIRequestContext, type Locator, type Page } from '@playwright/test'

// Only the fields the flows read; the web UI's generated client is the contract, this is a test's view of it
export interface Workspace {
  id: string
  version: number
  status: string
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

  put<T>(path: string, data: unknown): Promise<T> {
    return this.call('PUT', path, data)
  }

  delete(path: string): Promise<void> {
    return this.call('DELETE', path)
  }

  // For a workspace the flow made through the UI and may already have deleted there
  async deleteWorkspaceIfThere(id: string): Promise<void> {
    const response = await this.request.get(`/api/v1/workspaces/${id}`)
    if (response.ok()) {
      const { version } = (await response.json()) as Workspace
      await this.delete(`/workspaces/${id}?version=${version}`)
    }
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

// The figure in an account's header, Core's balance as formatted; \D* keeps 0.00 from matching 10.00
const balance = (pattern: string) => new RegExp(`^Balance ${pattern}$`)
export const NEGATIVE = (figure: string) => balance(`\\D*[−-]\\D*${figure.replace('.', '\\.')}`)
export const POSITIVE = (figure: string) => balance(`[^−\\d-]*${figure.replace('.', '\\.')}`)

// A real finger through the browser: the rows react to touch pointers only, which a mouse never is. By default
// most of the row's width at once; `distance` in pixels and a pause between steps make a short or slow one.
// Each move lands about a frame (~16 ms) after the last, so a fast gesture is few, large `steps`
export async function swipe(
  page: Page,
  target: Locator,
  direction: 'left' | 'right',
  { distance, steps = 10, stepPause = 0 }: { distance?: number; steps?: number; stepPause?: number } = {},
): Promise<void> {
  const box = (await target.boundingBox())!
  const y = box.y + box.height / 2
  const travel = distance ?? box.width * 0.7
  const from = direction === 'left' ? box.x + box.width * 0.85 : box.x + box.width * 0.15
  const to = direction === 'left' ? from - travel : from + travel
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: from, y }] })
  for (let step = 1; step <= steps; step++) {
    const x = from + ((to - from) * step) / steps
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y }] })
    if (stepPause > 0) {
      await page.waitForTimeout(stepPause)
    }
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
}

// Held past the 500 ms a tile waits for, then lifted
export async function longPress(page: Page, target: Locator): Promise<void> {
  const box = (await target.boundingBox())!
  const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [point] })
  await page.waitForTimeout(700)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
}

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
