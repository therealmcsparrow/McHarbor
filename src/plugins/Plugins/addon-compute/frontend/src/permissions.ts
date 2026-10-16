// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The permission keys this plugin owns.
 *
 * A plugin declares its own keys and never touches the host's `PERM` mirror in
 * `@core/auth/permissionKeys` — that mirror is for host-owned capabilities, and
 * these arrive in the catalog through `adds_permissions` in `plugin.yaml`
 * instead. The same discipline applies for the same reason: a bare
 * `'compute.manage'` literal at a call site is a rename no tool will catch.
 *
 * Must stay in step with `adds_permissions` in `plugin.yaml` and `AddsPerms` in
 * `backend/main.go`.
 */
export const COMPUTE_PERM = {
  VIEW: 'compute.view',
  MANAGE: 'compute.manage',
  DELETE: 'compute.delete',
  IMAGES_UPLOAD: 'compute.images.upload',
  POOLS_BROWSE: 'compute.pools.browse',
} as const;

export type ComputePermission = (typeof COMPUTE_PERM)[keyof typeof COMPUTE_PERM];

/**
 * Keys owned by the two guest plugins.
 *
 * This plugin never enforces them — it has no guest routes — but the overview
 * links to guest pages, and a link to a page the user cannot open is worse than
 * no link. Referencing the keys here keeps those affordances honest without
 * this plugin depending on either guest plugin being installed.
 */
export const GUEST_PERM = {
  VM_VIEW: 'vm.view',
  VM_MANAGE: 'vm.manage',
  CT_VIEW: 'ct.view',
  CT_MANAGE: 'ct.manage',
} as const;
