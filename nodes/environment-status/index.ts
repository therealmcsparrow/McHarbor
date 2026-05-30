// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const environmentStatus: NodeDefinition = {
  key: 'environment-status',
  label: 'Environment Status',
  category: 'action',
  description: 'Check Docker environment reachability and daemon metadata',
  icon: 'IconPlugConnected',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
