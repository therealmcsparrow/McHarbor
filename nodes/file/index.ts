// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const file: NodeDefinition = {
  key: 'file',
  label: 'File',
  category: 'utility',
  description: 'Read or write a file in the workflow data directory',
  icon: 'IconFileText',
  configSchema: [
    {
      key: 'operation',
      label: 'Operation',
      type: 'select',
      required: true,
      default: 'read',
      options: [
        {
          value: 'read',
          label: 'Read',
        },
        {
          value: 'write',
          label: 'Write',
        },
      ],
    },
    {
      key: 'path',
      label: 'Path',
      type: 'text',
      required: true,
    },
    {
      key: 'encoding',
      label: 'Encoding',
      type: 'select',
      required: false,
      default: 'utf-8',
      options: [
        {
          value: 'utf-8',
          label: 'UTF-8',
        },
        {
          value: 'base64',
          label: 'Base64',
        },
      ],
    },
    {
      key: 'output_property',
      label: 'Output Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: {
        operation: 'read',
      },
    },
    {
      key: 'property',
      label: 'Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: {
        operation: 'write',
      },
    },
    {
      key: 'mode',
      label: 'Mode',
      type: 'select',
      required: false,
      default: 'overwrite',
      options: [
        {
          value: 'overwrite',
          label: 'Overwrite',
        },
        {
          value: 'append',
          label: 'Append',
        },
      ],
      showWhen: {
        operation: 'write',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
