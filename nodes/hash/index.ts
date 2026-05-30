// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const hash: NodeDefinition = {
  "key": "hash",
  "label": "Hash",
  "category": "utility",
  "description": "Hash a value using a selected algorithm",
  "icon": "IconFingerprint",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "algorithm",
      "label": "Algorithm",
      "type": "select",
      "required": false,
      "default": "sha256",
      "options": [
        {
          "value": "md5",
          "label": "MD5"
        },
        {
          "value": "sha1",
          "label": "SHA1"
        },
        {
          "value": "sha256",
          "label": "SHA256"
        },
        {
          "value": "sha512",
          "label": "SHA512"
        }
      ]
    },
    {
      "key": "output_property",
      "label": "Output Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
