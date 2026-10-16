// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { CLEAR_READ_CACHE, READ_CACHE } from './cachePolicy';

export async function clearReadCache(): Promise<void> {
  if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
    navigator.serviceWorker.controller?.postMessage({ type: CLEAR_READ_CACHE });
  }
  if (typeof caches !== 'undefined') {
    await caches.delete(READ_CACHE).catch(() => false);
  }
}
