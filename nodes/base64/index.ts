// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const base64: NodeDefinition = {
  "key": "base64",
  "label": "Base64",
  "category": "utility",
  "description": "Encode or decode a value with Base64",
  "icon": "IconLockCode",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "action",
      "label": "Action",
      "type": "select",
      "required": false,
      "default": "encode",
      "options": [
        {
          "value": "encode",
          "label": "Encode"
        },
        {
          "value": "decode",
          "label": "Decode"
        }
      ]
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output",
    "error"
  ]
};
