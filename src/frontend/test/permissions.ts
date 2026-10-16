// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';

let current: CurrentUserPermissions = { permissions: [], wildcard: false };
const listeners = new Set<(next: CurrentUserPermissions) => void>();

const bridge = {
  snapshot: () => current,
  subscribe: (listener: (next: CurrentUserPermissions) => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
};

export function grantPermissions(keys: string[] | '*'): void {
  current = keys === '*' ? { permissions: [], wildcard: true } : { permissions: [...keys], wildcard: false };
  window.McharborHost = { ...window.McharborHost, permissions: bridge };
  for (const listener of listeners) listener(current);
}

export function resetPermissions(): void {
  grantPermissions([]);
}
