// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sort: NodeDefinition = {
  "key": "sort",
  "label": "Sort",
  "category": "logic",
  "description": "Sort an array on the current message",
  "icon": "IconSortAscending",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "sort_by",
      "label": "Sort By",
      "type": "text",
      "required": false
    },
    {
      "key": "direction",
      "label": "Direction",
      "type": "select",
      "required": false,
      "default": "asc",
      "options": [
        {
          "value": "asc",
          "label": "Ascending"
        },
        {
          "value": "desc",
          "label": "Descending"
        }
      ]
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
