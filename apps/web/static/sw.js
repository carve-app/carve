const CACHE = 'carve-v1';
const APP_SHELL = [
  '/',
  '/review',
  '/cards',
  '/decks',
  '/library',
  '/stats',
  '/settings',
  '/manifest.webmanifest',
];

self.addEventListener('install', (e) => {
  e.waitUntil(
    caches.open(CACHE).then((c) => c.addAll(APP_SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (e) => {
  const { request } = e;
  const url = new URL(request.url);

  // Never intercept API calls or cross-origin requests — let them go to network.
  if (url.hostname !== self.location.hostname || url.pathname.startsWith('/v1/')) {
    return;
  }

  // Navigation requests: network-first with app shell fallback.
  if (request.mode === 'navigate') {
    e.respondWith(
      fetch(request).catch(() => caches.match('/'))
    );
    return;
  }

  // Static assets: cache-first.
  e.respondWith(
    caches.match(request).then((cached) => cached ?? fetch(request).then((resp) => {
      if (resp.ok) {
        const clone = resp.clone();
        caches.open(CACHE).then((c) => c.put(request, clone));
      }
      return resp;
    }))
  );
});
