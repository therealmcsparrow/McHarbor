// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The permission keys this plugin owns.
 *
 * Must stay in step with `adds_permissions` in `plugin.yaml` and `AddsPerms` in
 * `backend/main.go`. Referencing a bare `'ct.manage'` literal at a call site is
 * a rename no tool will catch, which is why these are constants.
 */
export const CT_PERM = {
  VIEW: 'ct.view',
  MANAGE: 'ct.manage',
  DELETE: 'ct.delete',
  /** A container console is a root shell inside that container. */
  CONSOLE: 'ct.console',
  BACKUP: 'ct.backup',
  RESTORE: 'ct.restore',
  /**
   * Turning off the user namespace, binding host paths, nesting. Never in a
   * role preset — each of these removes a boundary with the host itself.
   */
  PRIVILEGED: 'ct.privileged',
} as const;

/**
 * Keys owned by the Compute Foundation plugin.
 *
 * The create dialog reads pools, networks and rootfs images from it.
 * Referencing the key here keeps those affordances honest — a user without it
 * gets a disabled picker naming what they need, rather than an empty one.
 */
export const COMPUTE_PERM = {
  VIEW: 'compute.view',
  MANAGE: 'compute.manage',
} as const;
