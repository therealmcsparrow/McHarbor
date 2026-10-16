// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerAction: NodeDefinition = {
  key: 'container-action',
  label: 'Container Action',
  category: 'action',
  description: 'Start, stop, restart, remove, rename or wait for a container',
  icon: 'IconPlayerPlay',
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
      key: 'action',
      label: 'Action',
      type: 'select',
      required: true,
      default: 'start',
      options: [
        {
          value: 'start',
          label: 'Start',
        },
        {
          value: 'stop',
          label: 'Stop',
        },
        {
          value: 'restart',
          label: 'Restart',
        },
        {
          value: 'remove',
          label: 'Remove',
        },
        {
          value: 'rename',
          label: 'Rename',
        },
        {
          value: 'wait',
          label: 'Wait',
        },
      ],
    },
    {
      key: 'timeout',
      label: 'Timeout (seconds)',
      type: 'number',
      required: false,
      default: 10,
      showWhen: {
        action: ['stop', 'restart', 'wait'],
      },
    },
    {
      key: 'new_name',
      label: 'New Name',
      type: 'text',
      required: false,
      showWhen: {
        action: 'rename',
      },
    },
    {
      key: 'unlock_protected',
      label: 'Unlock Protected Container',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        action: 'rename',
      },
    },
    {
      key: 'condition',
      label: 'Condition',
      type: 'select',
      required: false,
      default: 'not-running',
      options: [
        {
          value: 'not-running',
          label: 'Not Running',
        },
        {
          value: 'next-exit',
          label: 'Next Exit',
        },
        {
          value: 'removed',
          label: 'Removed',
        },
      ],
      showWhen: {
        action: 'wait',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
