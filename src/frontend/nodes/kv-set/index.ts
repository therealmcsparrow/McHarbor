// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const kvSet: NodeDefinition = {
  "key": "kv-set",
  "label": "KV Set",
  "category": "utility",
  "description": "Write a value to the workflow key-value store",
  "icon": "IconDatabaseImport",
  "configSchema": [
    {
      "key": "key",
      "label": "Key",
      "type": "text",
      "required": true
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "ttl",
      "label": "TTL (seconds)",
      "type": "number",
      "required": false,
      "default": 0
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
