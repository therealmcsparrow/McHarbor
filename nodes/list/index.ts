// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const list: NodeDefinition = {
  key: 'list',
  label: 'List',
  category: 'action',
  description: 'List containers, images, networks, volumes or stacks in an environment',
  icon: 'IconList',
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
      key: 'show_all',
      label: 'Show All',
      type: 'toggle',
      required: false,
      default: true,
      showWhen: {
        resource: ['container', 'image'],
      },
    },
    {
      key: 'name_filter',
      label: 'Name Filter',
      type: 'text',
      required: false,
      showWhen: {
        resource: 'container',
      },
    },
    {
      key: 'filter_dangling',
      label: 'Only Dangling',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        resource: ['image', 'volume'],
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
