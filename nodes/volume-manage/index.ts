// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeManage: NodeDefinition = {
  key: 'volume-manage',
  label: 'Volume Manage',
  category: 'action',
  description: 'Create, remove, back up or restore a Docker volume',
  icon: 'IconDatabase',
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
          value: 'backup',
          label: 'Backup',
        },
        {
          value: 'restore',
          label: 'Restore',
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
          value: 'local',
          label: 'Local',
        },
      ],
      required: false,
      default: 'local',
      showWhen: {
        action: 'create',
      },
    },
    {
      key: 'labels',
      label: 'Labels',
      type: 'key-value',
      required: false,
      showWhen: {
        action: 'create',
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
      key: 'volume',
      label: 'Volume',
      type: 'volume-select',
      required: false,
      showWhen: {
        action: ['remove', 'backup', 'restore'],
      },
    },
    {
      key: 'backup_path',
      label: 'Backup File Path',
      type: 'text',
      required: false,
      showWhen: {
        action: ['backup', 'restore'],
      },
    },
    {
      key: 'overwrite',
      label: 'Overwrite Existing Files',
      type: 'toggle',
      required: false,
      default: false,
      showWhen: {
        action: 'restore',
      },
    },
    {
      key: 'helper_image',
      label: 'Helper Image',
      type: 'image-select',
      allowCustom: true,
      required: false,
      default: 'alpine:3.20',
      showWhen: {
        action: ['backup', 'restore'],
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
