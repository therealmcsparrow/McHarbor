// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// A version counter for "the CSS custom properties on <html> just changed".
//
// Most of the app never needs this: it consumes theme colour as a Tailwind
// class and the browser repaints on its own. The Themes tab is the
// exception — it renders the *values*, reading them back with
// `getComputedStyle`, which React has no way to invalidate. Switching a
// theme therefore left the variable list showing the previous theme's
// colours until something else forced a render.
//
// A stylesheet also lands asynchronously, so even a re-render triggered by
// the theme id would read the old sheet. The counter is bumped by
// `useApplyThemeTokens` *after* it has written the inline layer, which is
// the first moment the values are true.

import { useSyncExternalStore } from 'react';

let version = 0;
const listeners = new Set<() => void>();

/** Announce that the document's theme variables have been rewritten. */
export function bumpThemeTokenVersion() {
  version += 1;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function snapshot(): number {
  return version;
}

/**
 * Re-renders the caller whenever the theme variables are rewritten. Use it
 * as a dependency anywhere a component reads a token with
 * `getComputedStyle` instead of consuming it as a class.
 */
export function useThemeTokenVersion(): number {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}
