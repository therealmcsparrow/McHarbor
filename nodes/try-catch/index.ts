// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const tryCatch: NodeDefinition = {
  "key": "try-catch",
  "label": "Try Catch",
  "category": "logic",
  "description": "Guard downstream execution and route failures into a catch branch",
  "icon": "IconShieldCheck",
  "configSchema": [
    {
      "key": "error_property",
      "label": "Error Property",
      "type": "expression",
      "required": false,
      "default": "error"
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output",
    "catch"
  ]
};
