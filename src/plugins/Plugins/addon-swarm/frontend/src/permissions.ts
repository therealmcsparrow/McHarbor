// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * The permission keys this plugin owns.
 *
 * A plugin declares its own keys and never touches the host's `PERM` mirror in
 * `@core/auth/permissionKeys` — that mirror is for host-owned capabilities,
 * and these arrive in the catalog through `adds_permissions` in `plugin.yaml`
 * instead. Same discipline as the host's mirror, and for the same reason: a
 * bare `'swarm_nodes.manage'` literal at a call site is a rename no tool will
 * ever catch.
 *
 * The *strings* are the ones the host used, unchanged, so a role granted
 * `swarm_services.scale` before Swarm became an add-on still grants it after.
 * The constant is exported as `PERM` rather than `SWARM_PERM` so the thirteen
 * ported call sites read exactly as they did in the host module — the object
 * they resolve against is the only thing that moved.
 *
 * Must stay in step with `adds_permissions` in `plugin.yaml` and `AddsPerms`
 * in `backend/main.go`.
 */
export const PERM = {
  SWARM_VIEW: 'swarm.view',
  SWARM_MANAGE: 'swarm.manage',
  SWARM_SERVICES_VIEW: 'swarm_services.view',
  SWARM_SERVICES_MANAGE: 'swarm_services.manage',
  SWARM_SERVICES_SCALE: 'swarm_services.scale',
  SWARM_SERVICES_DELETE: 'swarm_services.delete',
  SWARM_TASKS_VIEW: 'swarm_tasks.view',
  SWARM_NODES_VIEW: 'swarm_nodes.view',
  SWARM_NODES_MANAGE: 'swarm_nodes.manage',
  SWARM_NODES_DELETE: 'swarm_nodes.delete',
  SWARM_SECRETS_VIEW: 'swarm_secrets.view',
  SWARM_SECRETS_MANAGE: 'swarm_secrets.manage',
  SWARM_SECRETS_DELETE: 'swarm_secrets.delete',
  SWARM_CONFIGS_VIEW: 'swarm_configs.view',
  SWARM_CONFIGS_MANAGE: 'swarm_configs.manage',
  SWARM_CONFIGS_DELETE: 'swarm_configs.delete',
  SWARM_STACKS_VIEW: 'swarm_stacks.view',
  SWARM_STACKS_MANAGE: 'swarm_stacks.manage',
  SWARM_STACKS_DELETE: 'swarm_stacks.delete',
  SWARM_NETWORKS_VIEW: 'swarm_networks.view',
} as const;

export type SwarmPermission = (typeof PERM)[keyof typeof PERM];
