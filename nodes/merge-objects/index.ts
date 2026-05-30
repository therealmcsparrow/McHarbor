// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const mergeObjects: NodeDefinition = {
  "key": "merge-objects",
  "label": "Merge Objects",
  "category": "utility",
  "description": "Merge multiple source objects into one output",
  "icon": "IconArrowMergeBoth",
  "configSchema": [
    {
      "key": "sources",
      "label": "Sources",
      "type": "text",
      "required": true
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
    "output"
  ]
};
