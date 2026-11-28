const CACHE_NAME = 'rental-erp-shell-v1'
const SHELL_URLS = ['/', '/manifest.webmanifest', '/icon.svg']

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CACHE_NAME)
      .then((cache) => cache.addAll(SHELL_URLS))
      .then(() => self.skipWaiting()),
  )
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))),
      )
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  // Jangan pernah cache panggilan API agar sesi & status pengiriman selalu segar.
  if (url.pathname.startsWith('/api/')) return

  event.respondWith(
    fetch(req)
      .then((res) => {
        if (res.ok && url.origin === self.location.origin) {
          const copy = res.clone()
          caches.open(CACHE_NAME).then((cache) => cache.put(req, copy))
        }
        return res
      })
      .catch(() => caches.match(req)),
  )
})
