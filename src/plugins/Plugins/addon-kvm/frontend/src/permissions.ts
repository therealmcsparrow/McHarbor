// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const VM_PERM = {
  VIEW: 'vm.view',
  MANAGE: 'vm.manage',
  DELETE: 'vm.delete',
  CONSOLE: 'vm.console',
  MIGRATE: 'vm.migrate',
  PRIVILEGED: 'vm.privileged',
  IMPORT: 'vm.import',
  BACKUP_VIEW: 'vm.backup.view',
  BACKUP_MANAGE: 'vm.backup.manage',
  BACKUP_DELETE: 'vm.backup.delete',
  RESTORE_RUN: 'vm.restore.run',
} as const;

/** Host keys this plugin consumes; the create wizard reads vault secret names. */
export const SECRETS_PERM = {
  VIEW: 'secrets.view',
} as const;

export const COMPUTE_PERM = {
  VIEW: 'compute.view',
  MANAGE: 'compute.manage',
} as const;
