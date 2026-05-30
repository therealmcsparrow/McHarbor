// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dockerAction: NodeDefinition = {
  "key": "docker-action",
  "label": "Docker Action",
  "category": "action",
  "description": "Run a basic Docker container action",
  "icon": "IconBox",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "container",
      "label": "Container",
      "type": "container-select",
      "required": true
    },
    {
      "key": "action",
      "label": "Action",
      "type": "select",
      "required": true,
      "options": [
        {
          "value": "start",
          "label": "Start"
        },
        {
          "value": "stop",
          "label": "Stop"
        },
        {
          "value": "restart",
          "label": "Restart"
        },
        {
          "value": "remove",
          "label": "Remove"
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
