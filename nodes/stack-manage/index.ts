// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stackManage: NodeDefinition = {
  key: 'stack-manage',
  label: 'Stack Manage',
  category: 'action',
  description: 'Deploy, start, stop or remove a compose stack',
  icon: 'IconStack2',
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
      default: 'deploy',
      options: [
        {
          value: 'deploy',
          label: 'Deploy',
        },
        {
          value: 'up',
          label: 'Start (compose up)',
        },
        {
          value: 'down',
          label: 'Stop (compose down)',
        },
        {
          value: 'remove',
          label: 'Remove',
        },
      ],
    },
    {
      key: 'stack_name',
      label: 'Stack Name',
      type: 'stack-select',
      required: true,
      allowCustom: true,
    },
    {
      key: 'compose_source',
      label: 'Compose Source',
      type: 'select',
      required: false,
      default: 'message',
      options: [
        {
          value: 'message',
          label: 'Message',
        },
        {
          value: 'inline',
          label: 'Inline YAML',
        },
        {
          value: 'file',
          label: 'File Path',
        },
      ],
      showWhen: {
        action: 'deploy',
      },
    },
    {
      key: 'compose_content',
      label: 'Compose Content',
      type: 'textarea',
      required: false,
      showWhen: {
        action: 'deploy',
        compose_source: 'inline',
      },
    },
    {
      key: 'compose_path',
      label: 'Compose Path',
      type: 'text',
      required: false,
      showWhen: {
        action: 'deploy',
        compose_source: 'file',
      },
    },
    {
      key: 'remove_volumes',
      label: 'Remove Volumes',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        action: ['down', 'remove'],
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
