// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const volumeCreate: NodeDefinition = {
  "key": "volume-create",
  "label": "Volume Create",
  "category": "action",
  "description": "Create a Docker volume",
  "icon": "IconDatabasePlus",
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
      "key": "driver",
      "label": "Driver",
      "type": "text",
      "required": false,
      "default": "local"
    },
    {
      "key": "labels",
      "label": "Labels",
      "type": "key-value",
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
