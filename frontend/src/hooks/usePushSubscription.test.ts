import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    getVapidPublicKey: vi.fn(),
    listPushSubscriptions: vi.fn(),
    createPushSubscription: vi.fn(),
    deletePushSubscription: vi.fn(),
    sendTestPush: vi.fn(),
  }
})

vi.mock('../utils/vapidCache', () => ({
  cacheVapidKey: vi.fn().mockResolvedValue(undefined),
  readVapidKey: vi.fn().mockResolvedValue(null),
}))

import {
  ApiError,
  getVapidPublicKey,
  listPushSubscriptions,
  createPushSubscription,
  deletePushSubscription,
  sendTestPush,
} from '../api/client'
import { usePushSubscription } from './usePushSubscription'

const mockGetVapid = vi.mocked(getVapidPublicKey)
const mockListSubs = vi.mocked(listPushSubscriptions)
const mockCreateSub = vi.mocked(createPushSubscription)
const mockDeleteSub = vi.mocked(deletePushSubscription)
const mockSendTest = vi.mocked(sendTestPush)

interface MockSubscription {
  endpoint: string
  unsubscribe: ReturnType<typeof vi.fn>
  toJSON: () => { keys: { p256dh: string; auth: string }; endpoint: string }
}

interface MockState {
  permission: NotificationPermission
  permissionRequest: NotificationPermission
  subscription: MockSubscription | null
  subscribeImpl: () => Promise<MockSubscription>
}

let mockState: MockState

function makeSubscription(endpoint = 'https://push.example/endpoint/1'): MockSubscription {
  return {
    endpoint,
    unsubscribe: vi.fn().mockResolvedValue(true),
    toJSON: () => ({ endpoint, keys: { p256dh: 'p256dh-x', auth: 'auth-x' } }),
  }
}

function installNavigatorMocks() {
  mockState = {
    permission: 'default',
    permissionRequest: 'granted',
    subscription: null,
    subscribeImpl: async () => {
      const sub = makeSubscription()
      mockState.subscription = sub
      return sub
    },
  }

  const pushManager = {
    getSubscription: vi.fn(async () => mockState.subscription),
    subscribe: vi.fn(async () => mockState.subscribeImpl()),
  }

  Object.defineProperty(globalThis, 'PushManager', {
    configurable: true,
    value: function PushManager() {},
  })

  // jsdom does not implement the Notification API.
  const NotificationStub = function Notification() {} as unknown as typeof Notification
  Object.defineProperty(NotificationStub, 'permission', {
    configurable: true,
    get: () => mockState.permission,
  })
  Object.defineProperty(NotificationStub, 'requestPermission', {
    configurable: true,
    value: vi.fn(async () => {
      mockState.permission = mockState.permissionRequest
      return mockState.permissionRequest
    }),
  })
  Object.defineProperty(globalThis, 'Notification', {
    configurable: true,
    value: NotificationStub,
  })

  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: {
      ready: Promise.resolve({
        active: { postMessage: vi.fn() },
        pushManager,
      }),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      controller: null,
    },
  })

  Object.defineProperty(navigator, 'userAgent', {
    configurable: true,
    value: 'TestAgent/1.0',
  })
}

function uninstallNavigatorMocks() {
  delete (globalThis as { PushManager?: unknown }).PushManager
  delete (globalThis as { Notification?: unknown }).Notification
  Reflect.deleteProperty(navigator, 'serviceWorker')
}

beforeEach(() => {
  vi.clearAllMocks()
  installNavigatorMocks()
})

afterEach(() => {
  uninstallNavigatorMocks()
})

describe('usePushSubscription', () => {
  it('reports unsupported when service worker / PushManager / Notification missing', async () => {
    uninstallNavigatorMocks()
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: undefined,
    })
    const { result } = renderHook(() => usePushSubscription())
    expect(result.current.isSupported).toBe(false)
    expect(result.current.state).toBe('unsupported')
    // No API calls should be made
    expect(mockGetVapid).not.toHaveBeenCalled()
  })

  it('reports not-configured when VAPID endpoint returns 503', async () => {
    mockGetVapid.mockRejectedValue(new ApiError(503, 'push notifications not configured'))
    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.state).toBe('not-configured'))
    expect(result.current.vapidKey).toBeNull()
    expect(result.current.devices).toEqual([])
  })

  it('reports denied when Notification.permission is denied', async () => {
    mockState.permission = 'denied'
    mockGetVapid.mockResolvedValue({ public_key: 'BPUBKEY' })
    mockListSubs.mockResolvedValue([])
    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.state).toBe('denied'))
    expect(result.current.vapidKey).toBe('BPUBKEY')
  })

  it('subscribes successfully and transitions to subscribed', async () => {
    mockGetVapid.mockResolvedValue({ public_key: 'BPUBKEY' })
    mockListSubs.mockResolvedValue([])
    mockState.permissionRequest = 'granted'
    mockCreateSub.mockResolvedValue({
      id: 42,
      user_id: 1,
      endpoint: 'https://push.example/endpoint/1',
      p256dh: 'p256dh-x',
      auth: 'auth-x',
      user_agent: 'TestAgent/1.0',
      created_at: '2026-05-03T00:00:00Z',
    })

    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.vapidKey).toBe('BPUBKEY'))
    await waitFor(() => expect(result.current.state).toBe('unsubscribed'))

    await act(async () => {
      await result.current.subscribe()
    })

    expect(mockCreateSub).toHaveBeenCalledWith({
      endpoint: 'https://push.example/endpoint/1',
      keys: { p256dh: 'p256dh-x', auth: 'auth-x' },
      user_agent: 'TestAgent/1.0',
    })
    expect(result.current.state).toBe('subscribed')
    expect(result.current.devices).toHaveLength(1)
    expect(result.current.devices[0]!.id).toBe(42)
  })

  it('throws and transitions to denied when permission is denied during subscribe', async () => {
    mockGetVapid.mockResolvedValue({ public_key: 'BPUBKEY' })
    mockListSubs.mockResolvedValue([])
    mockState.permission = 'default'
    mockState.permissionRequest = 'denied'

    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.vapidKey).toBe('BPUBKEY'))
    await waitFor(() => expect(result.current.state).toBe('unsubscribed'))

    // React 19's act() rolls back state updates if the inner async function rejects,
    // so consume the error inside act() and assert on it after.
    let caught: unknown
    await act(async () => {
      caught = await result.current.subscribe().catch((e: unknown) => e)
    })
    expect((caught as Error).message).toMatch(/denied/i)
    expect(result.current.state).toBe('denied')
    expect(mockCreateSub).not.toHaveBeenCalled()
  })

  it('unsubscribes the current subscription and removes the matching backend row', async () => {
    const existingSub = makeSubscription('https://push.example/endpoint/2')
    mockState.subscription = existingSub
    mockGetVapid.mockResolvedValue({ public_key: 'BPUBKEY' })
    mockListSubs.mockResolvedValue([
      {
        id: 7,
        user_id: 1,
        endpoint: 'https://push.example/endpoint/2',
        p256dh: 'p256dh-x',
        auth: 'auth-x',
        user_agent: 'TestAgent/1.0',
        created_at: '2026-05-03T00:00:00Z',
      },
    ])
    mockDeleteSub.mockResolvedValue(undefined)

    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.state).toBe('subscribed'))

    await act(async () => {
      await result.current.unsubscribe()
    })

    expect(existingSub.unsubscribe).toHaveBeenCalled()
    expect(mockDeleteSub).toHaveBeenCalledWith(7)
    expect(result.current.state).toBe('unsubscribed')
    expect(result.current.devices).toEqual([])
  })

  it('sendTest calls the backend test endpoint', async () => {
    mockGetVapid.mockResolvedValue({ public_key: 'BPUBKEY' })
    mockListSubs.mockResolvedValue([])
    mockSendTest.mockResolvedValue({ sent: 1 })
    const { result } = renderHook(() => usePushSubscription())
    await waitFor(() => expect(result.current.state).toBe('unsubscribed'))

    await act(async () => {
      await result.current.sendTest()
    })

    expect(mockSendTest).toHaveBeenCalledOnce()
  })
})
