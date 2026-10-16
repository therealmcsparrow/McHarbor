// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const environmentInfo: NodeDefinition = {
  key: 'environment-info',
  label: 'Environment Info',
  category: 'action',
  description: 'Read Docker daemon, host or reachability information for an environment',
  icon: 'IconServer',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'scope',
      label: 'Information',
      type: 'select',
      required: true,
      default: 'docker',
      options: [
        {
          value: 'docker',
          label: 'Docker daemon',
        },
        {
          value: 'host',
          label: 'Host',
        },
        {
          value: 'status',
          label: 'Reachability',
        },
      ],
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
