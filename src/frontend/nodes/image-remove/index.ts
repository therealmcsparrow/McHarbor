// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const imageRemove: NodeDefinition = {
  "key": "image-remove",
  "label": "Image Remove",
  "category": "action",
  "description": "Remove a Docker image",
  "icon": "IconTrashX",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "image",
      "label": "Image",
      "type": "text",
      "required": true
    },
    {
      "key": "force",
      "label": "Force",
      "type": "toggle",
      "required": false,
      "default": false
    },
    {
      "key": "prune_children",
      "label": "Prune Children",
      "type": "toggle",
      "required": false,
      "default": true
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
