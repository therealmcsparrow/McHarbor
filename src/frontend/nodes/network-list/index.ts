// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const networkList: NodeDefinition = {
  "key": "network-list",
  "label": "Network List",
  "category": "action",
  "description": "List Docker networks",
  "icon": "IconNetwork",
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
