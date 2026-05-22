// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const join: NodeDefinition = {
  "key": "join",
  "label": "Join",
  "category": "logic",
  "description": "Join multiple incoming branches into a single message",
  "icon": "IconArrowMerge",
  "configSchema": [
    {
      "key": "input_count",
      "label": "Input Count",
      "type": "number",
      "required": false,
      "default": 2
    },
    {
      "key": "combine_mode",
      "label": "Combine Mode",
      "type": "select",
      "required": false,
      "default": "last",
      "options": [
        {
          "value": "last",
          "label": "Last Message"
        },
        {
          "value": "merge",
          "label": "Merge Objects"
        },
        {
          "value": "array",
          "label": "Array"
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
