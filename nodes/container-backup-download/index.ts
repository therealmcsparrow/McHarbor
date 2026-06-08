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
    { key: 'run_id', label: 'Run ID', type: 'text', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
