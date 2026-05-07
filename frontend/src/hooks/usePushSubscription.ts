import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ApiError,
  createPushSubscription,
  deletePushSubscription,
  getVapidPublicKey,
  listPushSubscriptions,
  sendTestPush,
} from '../api/client'
import type { PushSubscriptionInfo } from '../types'
import { cacheVapidKey } from '../utils/vapidCache'

export type PushState =
  | 'unsupported'
  | 'not-configured'
  | 'denied'
  | 'unsubscribed'
  | 'subscribed'
  | 'subscribing'

export interface UsePushSubscriptionResult {
  state: PushState
  devices: PushSubscriptionInfo[]
  vapidKey: string | null
  isSupported: boolean
  loadError: string | null
  subscribe: () => Promise<void>
  unsubscribe: () => Promise<void>
  removeDevice: (id: number) => Promise<void>
  sendTest: () => Promise<void>
  reload: () => Promise<void>
}

export function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4)
  const padded = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(padded)
  const buffer = new ArrayBuffer(raw.length)
  const out = new Uint8Array(buffer)
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

function detectSupport(): boolean {
  return (
    typeof navigator !== 'undefined' &&
    'serviceWorker' in navigator &&
    typeof window !== 'undefined' &&
    'PushManager' in window &&
    'Notification' in window
  )
}

async function postKeyToServiceWorker(key: string): Promise<void> {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return
  try {
    const reg = await navigator.serviceWorker.ready
    reg.active?.postMessage({ type: 'vapid-key', key })
  } catch {
    // ignore — the cached copy will still be readable in-place
  }
  await cacheVapidKey(key)
}

export function usePushSubscription(): UsePushSubscriptionResult {
  const isSupported = detectSupport()
  const [state, setState] = useState<PushState>(isSupported ? 'unsubscribed' : 'unsupported')
  const [devices, setDevices] = useState<PushSubscriptionInfo[]>([])
  const [vapidKey, setVapidKey] = useState<string | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const computeState = useCallback(
    async (devicesList: PushSubscriptionInfo[]): Promise<PushState> => {
      if (!isSupported) return 'unsupported'
      if (Notification.permission === 'denied') return 'denied'
      try {
        const reg = await navigator.serviceWorker.ready
        const sub = await reg.pushManager.getSubscription()
        if (sub && devicesList.some((d) => d.endpoint === sub.endpoint)) {
          return 'subscribed'
        }
      } catch {
        // fall through
      }
      return 'unsubscribed'
    },
    [isSupported],
  )

  const reload = useCallback(async () => {
    if (!isSupported) {
      setState('unsupported')
      return
    }
    setLoadError(null)
    let key: string
    try {
      const res = await getVapidPublicKey()
      key = res.public_key
    } catch (err) {
      if (err instanceof ApiError && err.status === 503) {
        if (mounted.current) {
          setState('not-configured')
          setVapidKey(null)
          setDevices([])
        }
        return
      }
      if (mounted.current) {
        setLoadError(err instanceof Error ? err.message : 'Failed to load notification settings')
      }
      return
    }
    if (mounted.current) setVapidKey(key)
    void postKeyToServiceWorker(key)

    let list: PushSubscriptionInfo[] = []
    try {
      list = await listPushSubscriptions()
    } catch (err) {
      if (mounted.current) {
        setLoadError(err instanceof Error ? err.message : 'Failed to list subscriptions')
      }
      return
    }
    if (!mounted.current) return
    setDevices(list)
    const next = await computeState(list)
    if (mounted.current) setState(next)
  }, [isSupported, computeState])

  useEffect(() => {
    void reload()
  }, [reload])

  const subscribe = useCallback(async () => {
    if (!isSupported) throw new Error('Push notifications not supported')
    if (!vapidKey) throw new Error('Notification server is not configured')
    setState('subscribing')
    try {
      let permission = Notification.permission
      if (permission === 'default') {
        permission = await Notification.requestPermission()
      }
      if (permission !== 'granted') {
        const t = permission === 'denied' ? 'denied' : 'unsubscribed'
        if (mounted.current) setState(t)
        throw new Error('Notification permission denied')
      }
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(vapidKey),
      })
      const json = sub.toJSON()
      const p256dh = json.keys?.p256dh ?? ''
      const auth = json.keys?.auth ?? ''
      const created = await createPushSubscription({
        endpoint: sub.endpoint,
        keys: { p256dh, auth },
        user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
      })
      if (!mounted.current) return
      setDevices((prev) => {
        const without = prev.filter((d) => d.id !== created.id)
        return [...without, created]
      })
      setState('subscribed')
    } catch (err) {
      if (mounted.current) {
        const next = await computeState(devices)
        setState(next)
      }
      throw err
    }
  }, [isSupported, vapidKey, devices, computeState])

  const unsubscribe = useCallback(async () => {
    if (!isSupported) return
    const reg = await navigator.serviceWorker.ready
    const sub = await reg.pushManager.getSubscription()
    if (sub) {
      const endpoint = sub.endpoint
      try {
        await sub.unsubscribe()
      } catch {
        // continue and still try to delete from backend
      }
      const match = devices.find((d) => d.endpoint === endpoint)
      if (match) {
        try {
          await deletePushSubscription(match.id)
        } catch (err) {
          if (!(err instanceof ApiError && err.status === 404)) throw err
        }
        if (mounted.current) {
          setDevices((prev) => prev.filter((d) => d.id !== match.id))
        }
      }
    }
    if (mounted.current) setState('unsubscribed')
  }, [isSupported, devices])

  const removeDevice = useCallback(async (id: number) => {
    await deletePushSubscription(id)
    if (!mounted.current) return
    const remaining = devices.filter((d) => d.id !== id)
    setDevices(remaining)
    const next = await computeState(remaining)
    if (mounted.current) setState(next)
  }, [devices, computeState])

  const sendTest = useCallback(async () => {
    await sendTestPush()
  }, [])

  return {
    state,
    devices,
    vapidKey,
    isSupported,
    loadError,
    subscribe,
    unsubscribe,
    removeDevice,
    sendTest,
    reload,
  }
}
