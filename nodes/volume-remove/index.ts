// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeRemove: NodeDefinition = {
  "key": "volume-remove",
  "label": "Volume Remove",
  "category": "action",
  "description": "Remove a Docker volume",
  "icon": "IconDatabaseMinus",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "name",
      "label": "Name",
      "type": "text",
      "required": true
    },
    {
      "key": "force",
      "label": "Force",
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
