// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const loop: NodeDefinition = {
  "key": "loop",
  "label": "Loop",
  "category": "logic",
  "description": "Iterate over an array from the current message",
  "icon": "IconRepeat",
  "configSchema": [
    {
      "key": "items_field",
      "label": "Items Field",
      "type": "expression",
      "required": false,
      "default": "payload"
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "done"
  ]
};
