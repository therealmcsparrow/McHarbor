// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PermissionSelection } from '@resources/components/security/PermissionEditor';

/**
 * Flatten a PermissionEditor selection into the legacy string[]
 * shape the roles API expects.
 *
 * - Wildcard wins outright: `["*"]`.
 * - Otherwise every "grant" override becomes an entry. "deny"
 *   overrides are NOT persisted today — the backend has no
 *   deny semantics yet. The editor still surfaces them in the
 *   UI so admins can preview the future ABAC shape.
 */
export function flattenSelection(sel: PermissionSelection): string[] {
  if (sel.wildcard) return ['*'];
  const out: string[] = [];
  for (const [key, state] of Object.entries(sel.overrides)) {
    if (state === 'grant') out.push(key);
  }
  return out;
}

/** Rehydrate a legacy string[] into the editor's selection shape. */
export function selectionFromPermissions(perms: string[]): PermissionSelection {
  if (perms.includes('*')) {
    return { overrides: {}, wildcard: true };
  }
  const overrides: Record<string, 'grant' | 'deny'> = {};
  for (const p of perms) overrides[p] = 'grant';
  return { overrides, wildcard: false };
}
