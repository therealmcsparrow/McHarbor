// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const encode: NodeDefinition = {
  key: 'encode',
  label: 'Encode',
  category: 'utility',
  description: 'Base64 encode or decode, or hash a value',
  icon: 'IconLockCode',
  configSchema: [
    {
      key: 'algorithm',
      label: 'Algorithm',
      type: 'select',
      required: true,
      default: 'base64-encode',
      options: [
        {
          value: 'base64-encode',
          label: 'Base64 encode',
        },
        {
          value: 'base64-decode',
          label: 'Base64 decode',
        },
        {
          value: 'md5',
          label: 'MD5',
        },
        {
          value: 'sha1',
          label: 'SHA1',
        },
        {
          value: 'sha256',
          label: 'SHA256',
        },
        {
          value: 'sha512',
          label: 'SHA512',
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
      key: 'output_property',
      label: 'Output Property',
      type: 'expression',
      required: false,
      default: 'payload',
      showWhen: {
        algorithm: ['md5', 'sha1', 'sha256', 'sha512'],
      },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
