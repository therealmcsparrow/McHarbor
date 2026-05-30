// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dockerPrune: NodeDefinition = {
  "key": "docker-prune",
  "label": "Docker Prune",
  "category": "action",
  "description": "Remove unused Docker resources from an environment",
  "icon": "IconTrashX",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "resource",
      "label": "Resource",
      "type": "select",
      "required": false,
      "default": "all",
      "options": [
        {
          "value": "all",
          "label": "All"
        },
        {
          "value": "containers",
          "label": "Containers"
        },
        {
          "value": "images",
          "label": "Images"
        },
        {
          "value": "volumes",
          "label": "Volumes"
        },
        {
          "value": "networks",
          "label": "Networks"
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
