// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const networkRemove: NodeDefinition = {
  "key": "network-remove",
  "label": "Network Remove",
  "category": "action",
  "description": "Remove a Docker network",
  "icon": "IconNetworkOff",
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
