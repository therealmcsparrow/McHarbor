// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { PermissionsBridge } from '@core/plugins/hostBridge';

export type CurrentUserPermissions = {
  permissions: string[];
  wildcard: boolean;
};

export type SystemRestartResponse = {
  message: string;
  code: string;
  result: {
    containerId: string;
    containerName: string;
    scheduledAt: string;
  };
};

export const SYSTEM_RESTART_PERMISSION = 'system.manage';

const EMPTY: CurrentUserPermissions = { permissions: [], wildcard: false };

// ---------------------------------------------------------------------------
// Host bridge
//
// One query, one store, two readers.
//
// `usePermissionsBridgePublisher` — mounted once, by the host root — owns the
// only `/auth/me/permissions` request and pushes every result into the
// module-level store below. Everything else *reads* that store. The host's own
// components read it directly; a plugin bundle, which compiled its own copy of
// this module, reads the host's copy through `window.McharborHost.permissions`
// (the same channel i18n already uses).
//
// The consumer hook deliberately contains no react-query call. It is reached
// from `@resources/components/ui/Button` via `usePermissionGate`, so it ends up
// compiled into every plugin bundle that renders a gated button — and `useQuery`
// there would demand a QueryClient from whichever react-query instance the
// bundle linked against, which is not the one the plugin's own
// `QueryClientProvider` fills. `enabled: false` does not save it either:
// `useQuery` calls `useQueryClient()` before it looks at `enabled`, so the
// plugin page dies with "No QueryClient set" the moment a gated button renders.
// Reading a plain external store has no such requirement.
//
// `publisherActive` tells the two copies apart. It is only ever set in the
// host's module instance.
// ---------------------------------------------------------------------------

let publisherActive = false;
let current: CurrentUserPermissions = EMPTY;
const listeners = new Set<(permissions: CurrentUserPermissions) => void>();

function snapshot(): CurrentUserPermissions {
  return current;
}

function subscribe(listener: (permissions: CurrentUserPermissions) => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function publish(next: CurrentUserPermissions): void {
  if (next === current) return;
  current = next;
  for (const listener of listeners) listener(next);
}

/** This module's own store — what the publisher writes to. */
const LOCAL: PermissionsBridge = { snapshot, subscribe };

/**
 * Where this copy of the module reads from: its own store when it owns the
 * publisher, the host's store when it does not. Falls back to the local store
 * (empty until something publishes) so a bundle loaded before the host root has
 * mounted still renders instead of throwing.
 */
function source(): PermissionsBridge {
  if (publisherActive) return LOCAL;
  return window.McharborHost?.permissions ?? LOCAL;
}

/**
 * Mounted once by the host root. Owns the single `/auth/me/permissions` query
 * and mirrors every result onto `window.McharborHost.permissions` so plugin
 * bundles can read it without a request of their own.
 */
export function usePermissionsBridgePublisher(enabled = true): void {
  const { data } = useQuery({
    queryKey: ['auth', 'me', 'permissions'],
    queryFn: () => api.get<CurrentUserPermissions>('/auth/me/permissions').then((r) => r.data ?? EMPTY),
    enabled,
    staleTime: 60_000,
  });

  useEffect(() => {
    publisherActive = true;
    window.McharborHost = { ...window.McharborHost, permissions: { snapshot, subscribe } };
  }, []);

  useEffect(() => {
    publish(data ?? EMPTY);
  }, [data]);
}

/**
 * The current user's permission set, as last published by
 * `usePermissionsBridgePublisher` — the host's own store in the host, the
 * host's store read across the bundle boundary in a plugin.
 *
 * Deliberately react-query-free; see the note at the top of this file. Nothing
 * here fetches, so `enabled` no longer gates a request — it only answers
 * "should this caller see a permission set at all", which is what
 * `Header.tsx`'s `useCurrentUserPermissions(!!user)` was asking for.
 */
export function useCurrentUserPermissions(enabled = true): { data: CurrentUserPermissions | undefined } {
  const store = source();
  const permissions = useSyncExternalStore(store.subscribe, store.snapshot, store.snapshot);

  return { data: enabled ? permissions : undefined };
}

export function hasPermission(permissions: CurrentUserPermissions | undefined, permission: string): boolean {
  if (!permissions) {
    return false;
  }
  if (permissions.wildcard) {
    return true;
  }
  return permissions.permissions.includes(permission);
}
