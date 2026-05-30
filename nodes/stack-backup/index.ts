// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stackBackup: NodeDefinition = {
  key: 'stack-backup',
  label: 'Stack Backup',
  category: 'action',
  description: 'Export stack container metadata before risky operations',
  icon: 'IconDatabaseExport',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'stack_name',
      label: 'Stack Name',
      type: 'text',
      required: true,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
