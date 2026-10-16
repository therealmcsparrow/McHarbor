// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { toast } from 'sonner';
import i18n from '@core/i18n/i18n';

export const SERVICE_WORKER_URL = '/sw.js';
export const SERVICE_WORKER_UNAVAILABLE = 'service-worker-unavailable';

export function serviceWorkerSupported(): boolean {
  return typeof navigator !== 'undefined' && 'serviceWorker' in navigator && window.isSecureContext;
}

export function whenLoaded(run: () => void): void {
  if (document.readyState === 'complete') {
    run();
    return;
  }
  window.addEventListener('load', () => run(), { once: true });
}

function register(): Promise<ServiceWorkerRegistration> {
  return navigator.serviceWorker.register(SERVICE_WORKER_URL, { scope: '/' });
}

export class ServiceWorkerUnavailableError extends Error {
  readonly detail: string | null;

  constructor(detail: string | null) {
    super(detail ? `${SERVICE_WORKER_UNAVAILABLE}: ${detail}` : SERVICE_WORKER_UNAVAILABLE);
    this.name = 'ServiceWorkerUnavailableError';
    this.detail = detail;
  }
}

export async function waitForServiceWorker(timeoutMs = 15_000): Promise<ServiceWorkerRegistration> {
  const existing = await navigator.serviceWorker.getRegistration('/');
  if (!existing) {
    try {
      await register();
    } catch (err) {
      throw new ServiceWorkerUnavailableError(err instanceof Error ? err.message : String(err));
    }
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new ServiceWorkerUnavailableError(null)), timeoutMs);
  });
  try {
    return await Promise.race([navigator.serviceWorker.ready, timeout]);
  } finally {
    clearTimeout(timer);
  }
}

export function registerServiceWorker(): void {
  if (!import.meta.env.PROD || !serviceWorkerSupported()) return;
  const hadController = Boolean(navigator.serviceWorker.controller);
  let announced = false;
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (!hadController || announced) return;
    announced = true;
    toast.info(i18n.t('oncall:update.ready'), {
      duration: Infinity,
      action: { label: i18n.t('oncall:update.reload'), onClick: () => window.location.reload() },
    });
  });
  whenLoaded(() => {
    register()
      .then((registration) => {
        document.addEventListener('visibilitychange', () => {
          if (document.visibilityState === 'visible') void registration.update().catch(() => undefined);
        });
      })
      .catch(() => undefined);
  });
}
