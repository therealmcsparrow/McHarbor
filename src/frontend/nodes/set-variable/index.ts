// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const setVariable: NodeDefinition = {
  "key": "set-variable",
  "label": "Set Variable",
  "category": "utility",
  "description": "Store a value on the flow context",
  "icon": "IconVariable",
  "configSchema": [
    {
      "key": "name",
      "label": "Variable Name",
      "type": "text",
      "required": true
    },
    {
      "key": "value",
      "label": "Value",
      "type": "json",
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
