// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const assert: NodeDefinition = {
  "key": "assert",
  "label": "Assert",
  "category": "logic",
  "description": "Assert that a condition is true and branch on the result",
  "icon": "IconShieldCheckFilled",
  "configSchema": [
    {
      "key": "field",
      "label": "Field",
      "type": "expression",
      "required": true,
      "default": "payload"
    },
    {
      "key": "operator",
      "label": "Operator",
      "type": "select",
      "required": true,
      "default": "==",
      "options": [
        {
          "value": "==",
          "label": "Equals"
        },
        {
          "value": "!=",
          "label": "Not equal"
        },
        {
          "value": "contains",
          "label": "Contains"
        },
        {
          "value": "regex",
          "label": "Matches regex"
        },
        {
          "value": "is_empty",
          "label": "Is empty"
        },
        {
          "value": "is_not_empty",
          "label": "Is not empty"
        }
      ]
    },
    {
      "key": "value",
      "label": "Value",
      "type": "text",
      "required": false
    },
    {
      "key": "message",
      "label": "Failure Message",
      "type": "text",
      "required": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "pass",
    "fail"
  ]
};
