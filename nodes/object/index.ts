// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const object: NodeDefinition = {
  key: 'object',
  label: 'Object',
  category: 'utility',
  description: 'Pick or omit fields, or map an object to a new shape',
  icon: 'IconTransform',
  configSchema: [
    {
      key: 'operation',
      label: 'Operation',
      type: 'select',
      required: true,
      default: 'pick',
      options: [
        {
          value: 'pick',
          label: 'Pick',
        },
        {
          value: 'omit',
          label: 'Omit',
        },
        {
          value: 'map',
          label: 'Map',
        },
      ],
    },
    {
      key: 'property',
      label: 'Property',
      type: 'expression',
      required: false,
      default: 'payload',
    },
    {
      key: 'fields',
      label: 'Fields',
      type: 'multiselect',
      allowCustom: true,
      required: false,
      showWhen: {
        operation: ['pick', 'omit'],
      },
    },
    {
      key: 'mapping',
      label: 'Mapping',
      type: 'key-value',
      required: false,
      showWhen: {
        operation: 'map',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
