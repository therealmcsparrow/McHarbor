// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const jsonPath: NodeDefinition = {
  "key": "json-path",
  "label": "JSON Path",
  "category": "utility",
  "description": "Read a value from structured data with a simple JSONPath expression",
  "icon": "IconRoute",
  "configSchema": [
    {
      "key": "expression",
      "label": "Expression",
      "type": "text",
      "required": true,
      "default": "$.payload"
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
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
