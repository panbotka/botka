import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

vi.mock('workbox-precaching', () => ({
  precacheAndRoute: vi.fn(),
  createHandlerBoundToURL: vi.fn(() => vi.fn()),
}))
vi.mock('workbox-routing', () => ({
  registerRoute: vi.fn(),
  NavigationRoute: vi.fn(),
}))
vi.mock('workbox-core', () => ({ clientsClaim: vi.fn() }))

const cacheVapidKey = vi.fn((_key: string) => Promise.resolve())
vi.mock('../utils/vapidCache', () => ({
  cacheVapidKey: (key: string) => cacheVapidKey(key),
  readVapidKey: vi.fn(() => Promise.resolve(null)),
}))

type Listener = (event: { data: unknown; waitUntil: (p: unknown) => void }) => void

const listeners = new Map<string, Listener>()
const skipWaiting = vi.fn(() => Promise.resolve())

/** Load sw.ts against a stubbed ServiceWorkerGlobalScope and return its `message` handler. */
async function loadMessageHandler(): Promise<Listener> {
  vi.stubGlobal('self', {
    __WB_MANIFEST: [],
    skipWaiting,
    addEventListener: (type: string, fn: Listener) => listeners.set(type, fn),
  })
  vi.resetModules()
  await import('./sw')
  const handler = listeners.get('message')
  if (!handler) throw new Error('sw.ts registered no message listener')
  return handler
}

function dispatch(handler: Listener, data: unknown) {
  handler({ data, waitUntil: () => {} })
}

describe('service worker message handler', () => {
  beforeEach(() => {
    listeners.clear()
    skipWaiting.mockClear()
    cacheVapidKey.mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('skips waiting on the SKIP_WAITING message workbox-window actually posts', async () => {
    const handler = await loadMessageHandler()
    dispatch(handler, { type: 'SKIP_WAITING' })
    expect(skipWaiting).toHaveBeenCalledTimes(1)
  })

  it('still skips waiting on the legacy skip-waiting message', async () => {
    const handler = await loadMessageHandler()
    dispatch(handler, { type: 'skip-waiting' })
    expect(skipWaiting).toHaveBeenCalledTimes(1)
  })

  it('caches the VAPID key without skipping waiting', async () => {
    const handler = await loadMessageHandler()
    dispatch(handler, { type: 'vapid-key', key: 'abc' })
    expect(cacheVapidKey).toHaveBeenCalledWith('abc')
    expect(skipWaiting).not.toHaveBeenCalled()
  })

  it('ignores unknown and empty messages', async () => {
    const handler = await loadMessageHandler()
    dispatch(handler, undefined)
    dispatch(handler, { type: 'navigate' })
    expect(skipWaiting).not.toHaveBeenCalled()
  })
})
