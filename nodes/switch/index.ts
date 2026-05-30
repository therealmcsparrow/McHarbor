// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const switchNode: NodeDefinition = {
  "key": "switch",
  "label": "Switch",
  "category": "logic",
  "description": "Route a message to dynamic case outputs",
  "icon": "IconArrowsSplit2",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "check_type",
      "label": "Check Type",
      "type": "select",
      "required": false,
      "default": "value",
      "options": [
        {
          "value": "value",
          "label": "Value"
        },
        {
          "value": "type",
          "label": "Type"
        },
        {
          "value": "regex",
          "label": "Regex"
        }
      ]
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "case_0",
    "default"
  ]
};
