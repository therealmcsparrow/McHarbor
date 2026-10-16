// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const parse: NodeDefinition = {
  key: 'parse',
  label: 'Parse',
  category: 'utility',
  description: 'Parse JSON, YAML, XML, CSV or HTML text into structured data',
  icon: 'IconBraces',
  configSchema: [
    {
      key: 'format',
      label: 'Format',
      type: 'select',
      required: true,
      default: 'json',
      options: [
        {
          value: 'json',
          label: 'JSON',
        },
        {
          value: 'yaml',
          label: 'YAML',
        },
        {
          value: 'xml',
          label: 'XML',
        },
        {
          value: 'csv',
          label: 'CSV',
        },
        {
          value: 'html',
          label: 'HTML',
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
      key: 'delimiter',
      label: 'Delimiter',
      type: 'select',
      allowCustom: true,
      options: [
        {
          value: ',',
          label: 'Comma',
        },
        {
          value: ';',
          label: 'Semicolon',
        },
        {
          value: '\\t',
          label: 'Tab',
        },
        {
          value: '|',
          label: 'Pipe',
        },
      ],
      required: false,
      default: ',',
      showWhen: {
        format: 'csv',
      },
    },
    {
      key: 'has_headers',
      label: 'Has Headers',
      type: 'toggle',
      required: false,
      default: true,
      showWhen: {
        format: 'csv',
      },
    },
    {
      key: 'selector',
      label: 'Selector',
      type: 'text',
      required: false,
      default: 'body',
      showWhen: {
        format: 'html',
      },
    },
    {
      key: 'output_type',
      label: 'Output Type',
      type: 'select',
      required: false,
      default: 'text',
      options: [
        {
          value: 'text',
          label: 'Text',
        },
        {
          value: 'html',
          label: 'HTML',
        },
        {
          value: 'attribute',
          label: 'Attribute',
        },
      ],
      showWhen: {
        format: 'html',
      },
    },
    {
      key: 'attribute',
      label: 'Attribute',
      type: 'text',
      required: false,
      showWhen: {
        format: 'html',
        output_type: 'attribute',
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
