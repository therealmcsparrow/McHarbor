// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dockerEventTrigger: NodeDefinition = {
  key: 'docker-event-trigger',
  label: 'Docker Event Trigger',
  category: 'trigger',
  description: 'Start a workflow from Docker events filtered by type, action, name, or label',
  icon: 'IconBellRinging',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'type',
      label: 'Event Type',
      type: 'select',
      required: false,
      default: 'any',
      options: [
        { value: 'any', label: 'Any' },
        { value: 'container', label: 'Container' },
        { value: 'image', label: 'Image' },
        { value: 'volume', label: 'Volume' },
        { value: 'network', label: 'Network' },
      ],
    },
    {
      key: 'action',
      label: 'Action',
      type: 'text',
      required: false,
    },
    {
      key: 'name',
      label: 'Name or ID',
      type: 'text',
      required: false,
    },
    {
      key: 'label',
      label: 'Label Filter',
      type: 'text',
      required: false,
    },
  ],
  inputPorts: [],
  outputPorts: ['output'],
};
