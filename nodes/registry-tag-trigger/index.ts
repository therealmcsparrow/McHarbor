// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const registryTagTrigger: NodeDefinition = {
  key: 'registry-tag-trigger',
  label: 'Registry Tag Trigger',
  category: 'trigger',
  description: 'Start a workflow when a watched image tag digest changes',
  icon: 'IconTag',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'image',
      label: 'Image Tag',
      type: 'text',
      required: true,
    },
    {
      key: 'registry_mode',
      label: 'Registry',
      type: 'select',
      required: true,
      default: 'custom',
      options: [
        { value: 'configured', label: 'Configured Registry' },
        { value: 'custom', label: 'Other' },
      ],
    },
    {
      key: 'registry_id',
      label: 'Registry',
      type: 'registry-select',
      required: false,
      showWhen: { registry_mode: 'configured' },
    },
    {
      key: 'interval',
      label: 'Interval (seconds)',
      type: 'number',
      required: false,
      default: 300,
    },
  ],
  inputPorts: [],
  outputPorts: ['output'],
};
