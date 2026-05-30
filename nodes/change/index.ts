// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const change: NodeDefinition = {
  "key": "change",
  "label": "Change",
  "category": "utility",
  "description": "Set or delete a value on msg, flow, or global scope",
  "icon": "IconEdit",
  "configSchema": [
    {
      "key": "action_type",
      "label": "Action",
      "type": "select",
      "required": false,
      "default": "set",
      "options": [
        {
          "value": "set",
          "label": "Set"
        },
        {
          "value": "delete",
          "label": "Delete"
        }
      ]
    },
    {
      "key": "scope",
      "label": "Scope",
      "type": "select",
      "required": false,
      "default": "msg",
      "options": [
        {
          "value": "msg",
          "label": "msg"
        },
        {
          "value": "flow",
          "label": "flow"
        },
        {
          "value": "global",
          "label": "global"
        }
      ]
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": true,
      "default": "payload"
    },
    {
      "key": "value",
      "label": "Value",
      "type": "json",
      "required": false,
      "showWhen": {
        "action_type": "set"
      }
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
