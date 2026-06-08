// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerStackLink: NodeDefinition = {
  key: 'container-stack-link',
  label: 'Link Container To Stack',
  category: 'action',
  description: 'Create or replace the manual stack link for a container',
  icon: 'IconGitBranch',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container', type: 'container-select', required: true },
    { key: 'stack_name', label: 'Stack Name', type: 'text', required: true },
    { key: 'service_name', label: 'Service Name', type: 'text', required: false },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
