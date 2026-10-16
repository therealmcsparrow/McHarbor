// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const networkManage: NodeDefinition = {
  key: 'network-manage',
  label: 'Network Manage',
  category: 'action',
  description: 'Create or remove a network, or connect and disconnect containers',
  icon: 'IconNetwork',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'action',
      label: 'Action',
      type: 'select',
      required: true,
      default: 'create',
      options: [
        {
          value: 'create',
          label: 'Create',
        },
        {
          value: 'remove',
          label: 'Remove',
        },
        {
          value: 'connect',
          label: 'Connect',
        },
        {
          value: 'disconnect',
          label: 'Disconnect',
        },
      ],
    },
    {
      key: 'name',
      label: 'Name',
      type: 'text',
      required: false,
      showWhen: {
        action: 'create',
      },
    },
    {
      key: 'driver',
      label: 'Driver',
      type: 'select',
      allowCustom: true,
      options: [
        {
          value: 'bridge',
          label: 'Bridge',
        },
        {
          value: 'overlay',
          label: 'Overlay',
        },
        {
          value: 'macvlan',
          label: 'Macvlan',
        },
        {
          value: 'ipvlan',
          label: 'IPvlan',
        },
        {
          value: 'host',
          label: 'Host',
        },
        {
          value: 'none',
          label: 'None',
        },
      ],
      required: false,
      default: 'bridge',
      showWhen: {
        action: 'create',
      },
    },
    {
      key: 'internal',
      label: 'Internal',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        action: 'create',
      },
    },
    {
      key: 'subnet',
      label: 'Subnet',
      type: 'text',
      required: false,
      showWhen: {
        action: 'create',
      },
    },
    {
      key: 'network',
      label: 'Network',
      type: 'network-select',
      required: false,
      showWhen: {
        action: ['remove', 'connect', 'disconnect'],
      },
    },
    {
      key: 'container',
      label: 'Container',
      type: 'container-select',
      required: false,
      showWhen: {
        action: ['connect', 'disconnect'],
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
