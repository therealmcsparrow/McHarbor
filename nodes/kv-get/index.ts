// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const kvGet: NodeDefinition = {
  "key": "kv-get",
  "label": "KV Get",
  "category": "utility",
  "description": "Read a value from the workflow key-value store",
  "icon": "IconDatabaseSearch",
  "configSchema": [
    {
      "key": "key",
      "label": "Key",
      "type": "text",
      "required": true
    },
    {
      "key": "output_property",
      "label": "Output Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "default_value",
      "label": "Default Value",
      "type": "text",
      "required": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
