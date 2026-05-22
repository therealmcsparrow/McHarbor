// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stackList: NodeDefinition = {
  "key": "stack-list",
  "label": "Stack List",
  "category": "action",
  "description": "List compose stacks by project label",
  "icon": "IconStack2",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
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
