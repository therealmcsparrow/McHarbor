// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useSyncExternalStore } from 'react';

/**
 * The environment the host currently has selected, from the topbar picker.
 *
 * The bridge has carried `snapshot` and `subscribe` since it was written; this
 * add-on family only ever used `list()`, and every page and dialog defaulted to
 * `environments[0]` instead. That is why creating a guest offered the wrong
 * host and why a second picker on a page could silently disagree with the one
 * in the topbar.
 *
 * Read through the bridge rather than by importing the host's store: this
 * bundle compiles its own copy of every module it imports, so importing the
 * store would create a second one that hydrates from the same key, looks right
 * on first paint, and then stops agreeing the moment the user switches.
 *
 * Empty string before the host has mounted, which callers already treat as
 * "no environment chosen yet".
 */
function bridge() {
  return window.McharborHost?.environment;
}

export function useHostEnvironment(): string {
  const subscribe = useCallback((onChange: () => void) => {
    const host = bridge();
    // No bridge means the value can never change, so an unsubscribe that does
    // nothing is honest rather than a polling fallback.
    if (!host?.subscribe) return () => undefined;
    return host.subscribe(() => onChange());
  }, []);

  return useSyncExternalStore(
    subscribe,
    () => bridge()?.snapshot?.() ?? '',
    () => '',
  );
}
