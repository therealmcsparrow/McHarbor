// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerBackupRun: NodeDefinition = {
  key: 'container-backup-run',
  label: 'Container Backup',
  category: 'action',
  description: 'Run an encrypted backup for a container',
  icon: 'IconDatabaseExport',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container', type: 'container-select', required: true },
    { key: 'storage_location_id', label: 'Storage Location', type: 'storage-location-select', required: false },
    { key: 'include_logs', label: 'Include Logs', type: 'toggle', required: false, default: false },
    { key: 'include_filesystem', label: 'Include Filesystem', type: 'toggle', required: false, default: false },
    { key: 'include_image', label: 'Include Image', type: 'toggle', required: false, default: false },
    { key: 'selected_mounts', label: 'Selected Mounts', type: 'text', required: false },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
