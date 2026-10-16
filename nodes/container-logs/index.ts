// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerLogs: NodeDefinition = {
  key: 'container-logs',
  label: 'Container Logs',
  category: 'action',
  description: 'Read recent logs from a container',
  icon: 'IconFileText',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'container',
      label: 'Container',
      type: 'container-select',
      required: true,
    },
    {
      key: 'tail',
      label: 'Tail Lines',
      type: 'number',
      required: false,
      default: 100,
    },
    {
      key: 'since',
      label: 'Since',
      type: 'select',
      allowCustom: true,
      options: [
        {
          value: '5m',
          label: 'Last 5 minutes',
        },
        {
          value: '15m',
          label: 'Last 15 minutes',
        },
        {
          value: '1h',
          label: 'Last hour',
        },
        {
          value: '6h',
          label: 'Last 6 hours',
        },
        {
          value: '24h',
          label: 'Last 24 hours',
        },
        {
          value: '168h',
          label: 'Last 7 days',
        },
      ],
      required: false,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
