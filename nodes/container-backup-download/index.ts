// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerBackupDownload: NodeDefinition = {
  key: 'container-backup-download',
  label: 'Backup Download Info',
  category: 'action',
  description: 'Validate a completed backup run and return its download metadata',
  icon: 'IconFileDownload',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container (filter)', type: 'container-select', required: false },
    { key: 'run_id', label: 'Backup Run', type: 'backup-run-select', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
