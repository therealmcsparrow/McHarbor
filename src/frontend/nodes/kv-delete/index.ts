// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const kvDelete: NodeDefinition = {
  "key": "kv-delete",
  "label": "KV Delete",
  "category": "utility",
  "description": "Delete a value from the workflow key-value store",
  "icon": "IconDatabaseX",
  "configSchema": [
    {
      "key": "key",
      "label": "Key",
      "type": "text",
      "required": true
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
