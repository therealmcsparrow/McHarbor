// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const math: NodeDefinition = {
  "key": "math",
  "label": "Math",
  "category": "utility",
  "description": "Apply a math operation to a numeric value",
  "icon": "IconMathFunction",
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
      "default": "add",
      "options": [
        {
          "value": "add",
          "label": "Add"
        },
        {
          "value": "subtract",
          "label": "Subtract"
        },
        {
          "value": "multiply",
          "label": "Multiply"
        },
        {
          "value": "divide",
          "label": "Divide"
        },
        {
          "value": "round",
          "label": "Round"
        },
        {
          "value": "floor",
          "label": "Floor"
        },
        {
          "value": "ceil",
          "label": "Ceil"
        },
        {
          "value": "abs",
          "label": "Absolute"
        },
        {
          "value": "min",
          "label": "Min"
        },
        {
          "value": "max",
          "label": "Max"
        },
        {
          "value": "modulo",
          "label": "Modulo"
        }
      ]
    },
    {
      "key": "operand",
      "label": "Operand",
      "type": "number",
      "required": false,
      "default": 0
    },
    {
      "key": "output_property",
      "label": "Output Property",
      "type": "expression",
      "required": false,
      "default": "payload"
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
