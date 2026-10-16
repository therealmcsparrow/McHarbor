// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useSyncExternalStore } from 'react';

/**
 * Table-or-card preference, remembered per page and per browser.
 *
 * The host keeps the equivalent environments preference in a Zustand store, but
 * this bundle has no zustand dependency and adding one would compile a second
 * copy into the UMD output. A module-level external store is the same contract,
 * and matches how `useHostEnvironment` already reads shared state here.
 */

export type ViewMode = 'table' | 'card';

const PREFIX = 'mcharbor-kvm-view:';

const modes = new Map<string, ViewMode>();
const listeners = new Set<() => void>();

function read(page: string): ViewMode {
  const cached = modes.get(page);
  if (cached) return cached;
  let stored: ViewMode = 'table';
  try {
    stored = window.localStorage.getItem(`${PREFIX}${page}`) === 'card' ? 'card' : 'table';
  } catch {
    // A blocked or full storage is not a reason to fail the page.
  }
  modes.set(page, stored);
  return stored;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** The persisted view mode for one page, and a setter every reader sees. */
export function useKvmView(page: string): {
  viewMode: ViewMode;
  setViewMode: (next: ViewMode) => void;
} {
  const snapshot = useCallback(() => read(page), [page]);
  const viewMode = useSyncExternalStore(subscribe, snapshot, snapshot);

  const setViewMode = useCallback(
    (next: ViewMode) => {
      if (next === read(page)) return;
      modes.set(page, next);
      try {
        window.localStorage.setItem(`${PREFIX}${page}`, next);
      } catch {
        // Same as above: remembering the choice is a convenience, not a contract.
      }
      listeners.forEach((listener) => listener());
    },
    [page],
  );

  return { viewMode, setViewMode };
}
