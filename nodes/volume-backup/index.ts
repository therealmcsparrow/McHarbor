// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeBackup: NodeDefinition = {
  key: 'volume-backup',
  label: 'Volume Backup',
  category: 'action',
  description: 'Archive a Docker volume to a host path using a helper container',
  icon: 'IconDatabaseExport',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'volume',
      label: 'Volume',
      type: 'text',
      required: true,
    },
    {
      key: 'backup_path',
      label: 'Backup File Path',
      type: 'text',
      required: true,
    },
    {
      key: 'helper_image',
      label: 'Helper Image',
      type: 'text',
      required: false,
      default: 'alpine:3.20',
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
