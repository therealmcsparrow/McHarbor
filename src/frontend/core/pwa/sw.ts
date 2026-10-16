// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/// <reference lib="webworker" />

import {
  CACHED_AT_HEADER,
  CLEAR_READ_CACHE,
  READ_CACHE,
  SHELL_CACHE_PREFIX,
  isReadCacheable,
  isSessionEnd,
  isShellAsset,
  isStoreableResponse,
} from './cachePolicy';

function isServiceWorkerScope(scope: unknown): scope is ServiceWorkerGlobalScope {
  return typeof scope === 'object' && scope !== null && 'registration' in scope && 'clients' in scope;
}

function serviceWorkerScope(scope: unknown): ServiceWorkerGlobalScope {
  if (!isServiceWorkerScope(scope)) throw new Error('sw.ts must run inside a service worker');
  return scope;
}

const sw = serviceWorkerScope(self);
declare const __MCHARBOR_PRECACHE__: { version: string; files: string[] };

const PRECACHE = __MCHARBOR_PRECACHE__;
const SHELL_CACHE = `${SHELL_CACHE_PREFIX}${PRECACHE.version}`;

type PushPayload = {
  title?: string;
  body?: string;
  severity?: string;
  url?: string;
  tag?: string;
};

sw.addEventListener('install', (evt) => {
  evt.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.addAll(PRECACHE.files))
      .then(() => sw.skipWaiting()),
  );
});

sw.addEventListener('activate', (evt) => {
  evt.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter((key) => key.startsWith(SHELL_CACHE_PREFIX) && key !== SHELL_CACHE)
            .map((key) => caches.delete(key)),
        ),
      )
      .then(() => sw.clients.claim()),
  );
});

sw.addEventListener('message', (evt) => {
  const data = evt.data as { type?: string } | null;
  if (data?.type === CLEAR_READ_CACHE) {
    evt.waitUntil(caches.delete(READ_CACHE));
  }
});

async function readThrough(request: Request): Promise<Response> {
  try {
    const res = await fetch(request);
    if (isSessionEnd(res)) {
      await caches.delete(READ_CACHE);
    } else if (isStoreableResponse(res)) {
      const headers = new Headers(res.headers);
      headers.set(CACHED_AT_HEADER, new Date().toISOString());
      const body = await res.clone().arrayBuffer();
      const cache = await caches.open(READ_CACHE);
      await cache.put(request, new Response(body, { status: 200, headers }));
    }
    return res;
  } catch (err) {
    const cached = await caches.match(request, { cacheName: READ_CACHE });
    if (cached) return cached;
    throw err;
  }
}

async function shellAsset(request: Request): Promise<Response> {
  const cached = await caches.match(request);
  if (cached) return cached;
  const res = await fetch(request);
  if (res.ok) {
    const cache = await caches.open(SHELL_CACHE);
    await cache.put(request, res.clone());
  }
  return res;
}

async function navigateOrShell(request: Request): Promise<Response> {
  try {
    return await fetch(request);
  } catch (err) {
    const shell = await caches.match('/index.html', { cacheName: SHELL_CACHE });
    if (shell) return shell;
    throw err;
  }
}

sw.addEventListener('fetch', (evt) => {
  const { request } = evt;
  const url = new URL(request.url);
  const originValue = sw.location.origin;
  if (request.method !== 'GET' || url.origin !== originValue) return;

  if (request.mode === 'navigate') {
    evt.respondWith(navigateOrShell(request));
    return;
  }
  if (isShellAsset(url, originValue)) {
    evt.respondWith(shellAsset(request));
    return;
  }
  if (isReadCacheable(request.method, url, originValue)) {
    evt.respondWith(readThrough(request));
  }
});

sw.addEventListener('push', (evt) => {
  let payload: PushPayload;
  try {
    payload = (evt.data?.json() ?? {}) as PushPayload;
  } catch {
    payload = { body: evt.data?.text() };
  }
  const critical = payload.severity === 'critical' || payload.severity === 'error';
  evt.waitUntil(
    sw.registration.showNotification(payload.title || 'McHarbor', {
      body: payload.body ?? '',
      tag: payload.tag || undefined,
      icon: '/api/pwa/icons/192.png',
      badge: '/api/pwa/icons/192.png',
      requireInteraction: critical,
      data: { url: payload.url || '/oncall' },
    }),
  );
});

sw.addEventListener('notificationclick', (evt) => {
  evt.notification.close();
  const raw = (evt.notification.data as { url?: string } | undefined)?.url ?? '/oncall';
  const target = raw.startsWith('/') && !raw.startsWith('//') ? raw : '/oncall';
  evt.waitUntil(
    sw.clients.matchAll({ type: 'window', includeUncontrolled: true }).then(async (windows) => {
      const existing = windows.find((w) => new URL(w.url).origin === sw.location.origin);
      if (existing) {
        await existing.focus();
        return existing
          .navigate(target)
          .catch(() => sw.clients.openWindow(target))
          .then(() => undefined);
      }
      return sw.clients.openWindow(target).then(() => undefined);
    }),
  );
});
