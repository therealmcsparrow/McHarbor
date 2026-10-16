// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Canonical list of legacy workflow-node action keys. Both the
 * client-side migration helper (`migrateWorkflow.ts`) and the
 * backend's `applyLegacyNodeAlias` agree on this set. Adding a new
 * entry requires updating the Go side too.
 *
 * Only host-to-host renames are listed. A key a plugin now owns (a
 * per-provider send node, a trigger that moved into a plugin) is claimed
 * by that plugin through a workflow-node-alias capability instead, so the
 * host names no provider.
 */
export const LEGACY_NODE_ALIASES: ReadonlySet<string> = new Set([
  'cron-trigger',
  'parse-json',
  'parse-yaml',
  'parse-xml',
  'parse-csv',
  'parse-html',
  'docker-action',
  'container-power',
  'container-rename',
  'container-wait',
  'container-inspect',
  'image-inspect',
  'network-inspect',
  'volume-inspect',
  'stack-status',
  'container-list',
  'image-list',
  'network-list',
  'volume-list',
  'stack-list',
  'kv-get',
  'kv-set',
  'kv-delete',
  'docker-info',
  'host-info',
  'environment-status',
  'base64',
  'hash',
  'pick',
  'omit',
  'map',
  'read-file',
  'write-file',
  'docker-image-pull',
  'image-push',
  'image-tag',
  'image-remove',
  'network-create',
  'network-remove',
  'network-connect',
  'volume-create',
  'volume-remove',
  'volume-backup',
  'volume-restore',
  'stack-deploy',
  'compose-up',
  'compose-down',
  'stack-remove',
  'container-status-trigger',
  'container-health-trigger',
]);

export function isLegacyNodeAction(action: string): boolean {
  return LEGACY_NODE_ALIASES.has(action);
}
