// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const map: NodeDefinition = {
  "key": "map",
  "label": "Map",
  "category": "utility",
  "description": "Map array object keys to a new shape",
  "icon": "IconTransform",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "mapping",
      "label": "Mapping",
      "type": "key-value",
      "required": false
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
