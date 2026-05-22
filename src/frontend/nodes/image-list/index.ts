// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const imageList: NodeDefinition = {
  "key": "image-list",
  "label": "Image List",
  "category": "action",
  "description": "List images in an environment",
  "icon": "IconPhoto",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "show_all",
      "label": "Show All",
      "type": "toggle",
      "required": false,
      "default": false
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
