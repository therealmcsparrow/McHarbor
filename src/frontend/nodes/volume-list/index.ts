// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeList: NodeDefinition = {
  "key": "volume-list",
  "label": "Volume List",
  "category": "action",
  "description": "List Docker volumes",
  "icon": "IconDatabase",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "filter_dangling",
      "label": "Only Dangling",
      "type": "toggle",
      "required": false,
      "default": false
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
