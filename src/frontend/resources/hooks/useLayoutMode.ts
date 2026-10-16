// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useSyncExternalStore } from 'react';
import { useLayoutModeStore, type LayoutMode } from '@resources/stores/layoutMode';

export const ONCALL_MEDIA_QUERY = '(max-width: 639px)';

function subscribe(onChange: () => void): () => void {
  if (typeof window.matchMedia !== 'function') return () => undefined;
  const query = window.matchMedia(ONCALL_MEDIA_QUERY);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

function narrowViewport(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia(ONCALL_MEDIA_QUERY).matches;
}

export function resolveLayout(mode: LayoutMode, narrow: boolean): 'oncall' | 'desktop' {
  if (mode === 'auto') return narrow ? 'oncall' : 'desktop';
  return mode;
}

export function useResolvedLayout(): 'oncall' | 'desktop' {
  const mode = useLayoutModeStore((s) => s.mode);
  const narrow = useSyncExternalStore(subscribe, narrowViewport, () => false);
  return resolveLayout(mode, narrow);
}
