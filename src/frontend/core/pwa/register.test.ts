// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { ServiceWorkerUnavailableError, waitForServiceWorker, whenLoaded } from './register';

describe('service worker registration timing', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('runs at once when the page finished loading before the app booted', () => {
    vi.spyOn(document, 'readyState', 'get').mockReturnValue('complete');
    const run = vi.fn();
    whenLoaded(run);
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('waits for load while the page is still loading, and runs once', () => {
    vi.spyOn(document, 'readyState', 'get').mockReturnValue('loading');
    const run = vi.fn();
    whenLoaded(run);
    expect(run).not.toHaveBeenCalled();
    window.dispatchEvent(new Event('load'));
    window.dispatchEvent(new Event('load'));
    expect(run).toHaveBeenCalledTimes(1);
  });

  it('fails instead of hanging when no worker ever becomes ready', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('navigator', {
      serviceWorker: {
        getRegistration: vi.fn().mockResolvedValue(undefined),
        register: vi.fn().mockResolvedValue({}),
        ready: new Promise(() => undefined),
      },
    });
    const pending = waitForServiceWorker(10_000);
    const settled = expect(pending).rejects.toThrow('service-worker-unavailable');
    await vi.advanceTimersByTimeAsync(10_001);
    await settled;
    vi.useRealTimers();
  });

  it("reports the browser's own reason at once when registration is refused", async () => {
    vi.stubGlobal('navigator', {
      serviceWorker: {
        getRegistration: vi.fn().mockResolvedValue(undefined),
        register: vi.fn().mockRejectedValue(new Error('An SSL certificate error occurred when fetching the script.')),
        ready: new Promise(() => undefined),
      },
    });
    const error = await waitForServiceWorker(60_000).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ServiceWorkerUnavailableError);
    expect((error as ServiceWorkerUnavailableError).detail).toBe(
      'An SSL certificate error occurred when fetching the script.',
    );
  });
});
