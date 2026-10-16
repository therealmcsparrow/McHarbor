// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const imageManage: NodeDefinition = {
  key: 'image-manage',
  label: 'Image Manage',
  category: 'action',
  description: 'Pull, push, tag or remove a Docker image',
  icon: 'IconPhoto',
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
      default: 'pull',
      options: [
        {
          value: 'pull',
          label: 'Pull',
        },
        {
          value: 'push',
          label: 'Push',
        },
        {
          value: 'tag',
          label: 'Tag',
        },
        {
          value: 'remove',
          label: 'Remove',
        },
      ],
    },
    {
      key: 'registry_mode',
      label: 'Registry',
      type: 'select',
      required: false,
      default: 'custom',
      options: [
        {
          value: 'configured',
          label: 'Configured Registry',
        },
        {
          value: 'custom',
          label: 'Other',
        },
      ],
      showWhen: {
        action: ['pull', 'push'],
      },
    },
    {
      key: 'registry_id',
      label: 'Registry',
      type: 'registry-select',
      required: false,
      showWhen: {
        action: ['pull', 'push'],
        registry_mode: 'configured',
      },
    },
    {
      key: 'registry_url',
      label: 'Registry URL',
      type: 'text',
      required: false,
      showWhen: {
        action: ['pull', 'push'],
        registry_mode: 'custom',
      },
    },
    {
      key: 'registry_username',
      label: 'Registry Username',
      type: 'text',
      required: false,
      showWhen: {
        action: ['pull', 'push'],
        registry_mode: 'custom',
      },
    },
    {
      key: 'registry_password',
      label: 'Registry Password',
      type: 'text',
      required: false,
      secret: true,
      showWhen: {
        action: ['pull', 'push'],
        registry_mode: 'custom',
      },
    },
    {
      key: 'image',
      label: 'Image',
      type: 'image-select',
      allowCustom: true,
      required: false,
      showWhen: {
        action: 'pull',
      },
    },
    {
      key: 'image',
      label: 'Image',
      type: 'image-select',
      required: false,
      showWhen: {
        action: ['push', 'remove'],
      },
      allowCustom: true,
    },
    {
      key: 'source',
      label: 'Source Image',
      type: 'image-select',
      required: false,
      showWhen: {
        action: 'tag',
      },
    },
    {
      key: 'target',
      label: 'Target Image',
      type: 'text',
      required: false,
      showWhen: {
        action: 'tag',
      },
    },
    {
      key: 'force',
      label: 'Force',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        action: 'remove',
      },
    },
    {
      key: 'prune_children',
      label: 'Prune Children',
      type: 'toggle',
      required: false,
      default: true,
      showWhen: {
        action: 'remove',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
