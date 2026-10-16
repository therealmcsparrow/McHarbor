// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const kv: NodeDefinition = {
  key: 'kv',
  label: 'KV Store',
  category: 'utility',
  description: 'Read, write or delete a value in the workflow key-value store',
  icon: 'IconDatabase',
  configSchema: [
    {
      key: 'operation',
      label: 'Operation',
      type: 'select',
      required: true,
      default: 'get',
      options: [
        {
          value: 'get',
          label: 'Get',
        },
        {
          value: 'set',
          label: 'Set',
        },
        {
          value: 'delete',
          label: 'Delete',
        },
      ],
    },
    {
      key: 'key',
      label: 'Key',
      type: 'text',
      required: true,
    },
    {
      key: 'output_property',
      label: 'Output Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: {
        operation: 'get',
      },
    },
    {
      key: 'default_value',
      label: 'Default Value',
      type: 'text',
      required: false,
      showWhen: {
        operation: 'get',
      },
    },
    {
      key: 'property',
      label: 'Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: {
        operation: 'set',
      },
    },
    {
      key: 'ttl',
      label: 'TTL (seconds)',
      type: 'number',
      required: false,
      default: 0,
      showWhen: {
        operation: 'set',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output'],
};
