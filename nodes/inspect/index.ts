// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const inspect: NodeDefinition = {
  key: 'inspect',
  label: 'Inspect',
  category: 'action',
  description: 'Inspect a container, image, network, volume or stack',
  icon: 'IconZoomScan',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'resource',
      label: 'Resource',
      type: 'select',
      required: true,
      default: 'container',
      options: [
        {
          value: 'container',
          label: 'Container',
        },
        {
          value: 'image',
          label: 'Image',
        },
        {
          value: 'network',
          label: 'Network',
        },
        {
          value: 'volume',
          label: 'Volume',
        },
        {
          value: 'stack',
          label: 'Stack',
        },
      ],
    },
    {
      key: 'container',
      label: 'Container',
      type: 'container-select',
      required: false,
      showWhen: {
        resource: 'container',
      },
    },
    {
      key: 'image',
      label: 'Image',
      type: 'image-select',
      required: false,
      showWhen: {
        resource: 'image',
      },
    },
    {
      key: 'network',
      label: 'Network',
      type: 'network-select',
      required: false,
      showWhen: {
        resource: 'network',
      },
    },
    {
      key: 'volume',
      label: 'Volume',
      type: 'volume-select',
      required: false,
      showWhen: {
        resource: 'volume',
      },
    },
    {
      key: 'stack_name',
      label: 'Stack Name',
      type: 'stack-select',
      required: false,
      showWhen: {
        resource: 'stack',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
