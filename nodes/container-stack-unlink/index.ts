// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerStackUnlink: NodeDefinition = {
  key: 'container-stack-unlink',
  label: 'Unlink Container From Stack',
  category: 'action',
  description: 'Remove the manual stack link for a container',
  icon: 'IconTopologyComplex',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container', type: 'container-select', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
