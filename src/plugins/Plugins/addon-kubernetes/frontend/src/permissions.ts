// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The permission keys this plugin owns.
 *
 * A plugin declares its own keys and never touches the host's `PERM` mirror in
 * `@core/auth/permissionKeys` — that mirror is for host-owned capabilities,
 * and this plugin's keys arrive in the catalog through `adds_permissions` in
 * `plugin.yaml` instead. Same discipline as the host's mirror, though, and for
 * the same reason: referencing a bare `'kubernetes.manage'` literal at a call
 * site is a rename no tool will ever catch.
 *
 * Must stay in step with `adds_permissions` in `plugin.yaml` and `AddsPerms`
 * in `backend/main.go`.
 */
export const KUBE_PERM = {
  VIEW: 'kubernetes.view',
  LOGS: 'kubernetes.logs',
  MANAGE: 'kubernetes.manage',
  DELETE: 'kubernetes.delete',
  CLUSTERS_MANAGE: 'kubernetes.clusters.manage',
} as const;

export type KubePermission = (typeof KUBE_PERM)[keyof typeof KUBE_PERM];
