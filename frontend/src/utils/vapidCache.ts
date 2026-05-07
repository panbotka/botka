// Stores the server's VAPID public key in Cache Storage so the
// service worker can re-subscribe on `pushsubscriptionchange` without
// hitting the network.

const CACHE_NAME = 'botka-push-config'
const KEY_URL = 'https://botka.invalid/vapid-public-key'

export async function cacheVapidKey(key: string): Promise<void> {
  if (typeof caches === 'undefined' || !key) return
  try {
    const cache = await caches.open(CACHE_NAME)
    await cache.put(
      KEY_URL,
      new Response(key, { headers: { 'Content-Type': 'text/plain' } }),
    )
  } catch {
    // Cache Storage may be unavailable (private mode, quota, etc.).
    // The next subscribe will simply re-fetch from the server.
  }
}

export async function readVapidKey(): Promise<string | null> {
  if (typeof caches === 'undefined') return null
  try {
    const cache = await caches.open(CACHE_NAME)
    const res = await cache.match(KEY_URL)
    if (!res) return null
    return await res.text()
  } catch {
    return null
  }
}
