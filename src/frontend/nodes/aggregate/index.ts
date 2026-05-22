// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const aggregate: NodeDefinition = {
  "key": "aggregate",
  "label": "Aggregate",
  "category": "logic",
  "description": "Collect items for later aggregation in the executor",
  "icon": "IconArrowMergeBoth",
  "configSchema": [
    {
      "key": "group_by",
      "label": "Group By",
      "type": "expression",
      "required": false
    },
    {
      "key": "mode",
      "label": "Mode",
      "type": "select",
      "required": false,
      "default": "list",
      "options": [
        {
          "value": "list",
          "label": "List"
        },
        {
          "value": "count",
          "label": "Count"
        },
        {
          "value": "sum",
          "label": "Sum"
        }
      ]
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
