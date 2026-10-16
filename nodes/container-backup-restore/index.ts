// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerBackupRestore: NodeDefinition = {
  key: 'container-backup-restore',
  label: 'Container Backup Restore',
  category: 'action',
  description: 'Restore a completed container backup run into the container',
  icon: 'IconDatabaseImport',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container (filter)', type: 'container-select', required: false },
    { key: 'run_id', label: 'Backup Run', type: 'backup-run-select', required: true },
    { key: 'restore_items', label: 'Restore Items', type: 'backup-restore-item-multiselect', required: false },
    { key: 'secret_key', label: 'Secret Key', type: 'text', required: false, secret: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
