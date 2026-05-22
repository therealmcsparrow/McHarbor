// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stringOps: NodeDefinition = {
  "key": "string-ops",
  "label": "String Ops",
  "category": "utility",
  "description": "Apply common string operations to a value",
  "icon": "IconLetterCase",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "operation",
      "label": "Operation",
      "type": "select",
      "required": false,
      "default": "uppercase",
      "options": [
        {
          "value": "uppercase",
          "label": "Uppercase"
        },
        {
          "value": "lowercase",
          "label": "Lowercase"
        },
        {
          "value": "trim",
          "label": "Trim"
        },
        {
          "value": "replace",
          "label": "Replace"
        },
        {
          "value": "split",
          "label": "Split"
        },
        {
          "value": "join",
          "label": "Join"
        },
        {
          "value": "substring",
          "label": "Substring"
        },
        {
          "value": "reverse",
          "label": "Reverse"
        }
      ]
    },
    {
      "key": "arg1",
      "label": "Arg 1",
      "type": "text",
      "required": false
    },
    {
      "key": "arg2",
      "label": "Arg 2",
      "type": "text",
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
