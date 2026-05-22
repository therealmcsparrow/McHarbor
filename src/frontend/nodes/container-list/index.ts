// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerList: NodeDefinition = {
  "key": "container-list",
  "label": "Container List",
  "category": "action",
  "description": "List containers in an environment",
  "icon": "IconLayoutList",
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
      "default": true
    },
    {
      "key": "name_filter",
      "label": "Name Filter",
      "type": "text",
      "required": false
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
