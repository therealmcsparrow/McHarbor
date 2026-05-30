// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeRestore: NodeDefinition = {
  key: 'volume-restore',
  label: 'Volume Restore',
  category: 'action',
  description: 'Restore a Docker volume from a tar archive using a helper container',
  icon: 'IconDatabaseImport',
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
      key: 'overwrite',
      label: 'Overwrite Existing Files',
      type: 'toggle',
      required: false,
      default: false,
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
