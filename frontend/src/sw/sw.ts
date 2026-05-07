/// <reference lib="webworker" />

import { precacheAndRoute, createHandlerBoundToURL } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { clientsClaim } from 'workbox-core'
import { cacheVapidKey, readVapidKey } from '../utils/vapidCache'

declare const self: ServiceWorkerGlobalScope & {
  __WB_MANIFEST: Array<{ url: string; revision: string | null }>
}

clientsClaim()

precacheAndRoute(self.__WB_MANIFEST)

// Mirror the previous workbox `navigateFallback` + denylist so SPA
// navigation still falls back to /index.html while /api and /mcp
// requests pass straight to the network.
registerRoute(
  new NavigationRoute(createHandlerBoundToURL('/index.html'), {
    denylist: [/^\/api\//, /^\/mcp\//],
  }),
)

interface PushPayload {
  title?: string
  body?: string
  url?: string
  tag?: string
}

self.addEventListener('message', (event) => {
  const data = event.data as { type?: string; key?: string } | undefined
  if (!data) return
  if (data.type === 'vapid-key' && typeof data.key === 'string') {
    event.waitUntil(cacheVapidKey(data.key))
    return
  }
  if (data.type === 'skip-waiting') {
    void self.skipWaiting()
  }
})

self.addEventListener('push', (event) => {
  // The OS deduplicates by `tag`, so when this fires while the same
  // tab is open the in-tab `useNotifications` hook and this SW
  // notification surface as a single user-visible alert.
  let payload: PushPayload | null = null
  if (event.data) {
    try {
      payload = event.data.json() as PushPayload
    } catch {
      payload = null
    }
  }

  const fallback = payload === null
  const title = payload?.title || 'Botka'
  const body = payload?.body ?? (fallback ? 'New notification' : '')
  const tag = payload?.tag
  const url = payload?.url || '/'

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      icon: '/android-chrome-192x192.png',
      badge: '/favicon-32x32.png',
      data: { url },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const data = event.notification.data as { url?: string } | null
  const url = data?.url || '/'

  event.waitUntil((async () => {
    const allClients = await self.clients.matchAll({
      type: 'window',
      includeUncontrolled: true,
    })
    for (const client of allClients) {
      try {
        await client.focus()
      } catch {
        continue
      }
      client.postMessage({ type: 'navigate', url })
      return
    }
    await self.clients.openWindow(url)
  })())
})

function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4)
  const padded = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(padded)
  const buffer = new ArrayBuffer(raw.length)
  const out = new Uint8Array(buffer)
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

self.addEventListener('pushsubscriptionchange', ((event: ExtendableEvent) => {
  event.waitUntil((async () => {
    const key = await readVapidKey()
    if (!key) return
    try {
      await self.registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(key),
      })
    } catch (err) {
      console.warn('[botka SW] resubscribe failed', err)
    }
  })())
}) as EventListener)
