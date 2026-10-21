// Minimal app-shell service worker.
// - Navigations: network first, falling back to the cached shell when offline.
// - /assets/* (content-hashed): stale-while-revalidate.
// - /api/*, /mcp and /health are never intercepted, so OIDC redirects,
//   server-sent events and auth cookies always reach the server untouched.

const CACHE = "ownership-shell-v1";
const SHELL_URL = "/index.html";
const BYPASS_PREFIXES = ["/api/", "/mcp", "/health"];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((cache) => cache.add(SHELL_URL)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (request.method !== "GET" || url.origin !== self.location.origin || isBypassed(url.pathname)) {
    return;
  }
  if (request.mode === "navigate") {
    event.respondWith(networkFirstShell(request));
    return;
  }
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(staleWhileRevalidate(request));
  }
});

function isBypassed(pathname) {
  return BYPASS_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(prefix));
}

async function networkFirstShell(request) {
  const cache = await caches.open(CACHE);
  try {
    const response = await fetch(request);
    if (response.ok) {
      cache.put(SHELL_URL, response.clone());
    }
    return response;
  } catch (error) {
    const cached = await cache.match(SHELL_URL);
    if (cached) {
      return cached;
    }
    throw error;
  }
}

async function staleWhileRevalidate(request) {
  const cache = await caches.open(CACHE);
  const cached = await cache.match(request);
  const network = fetch(request).then((response) => {
    if (response.ok) {
      cache.put(request, response.clone());
    }
    return response;
  });
  if (cached) {
    network.catch(() => undefined);
    return cached;
  }
  return network;
}
