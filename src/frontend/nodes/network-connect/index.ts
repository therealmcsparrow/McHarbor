// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const networkConnect: NodeDefinition = {
  "key": "network-connect",
  "label": "Network Connect",
  "category": "action",
  "description": "Connect or disconnect a container from a network",
  "icon": "IconPlugConnected",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "network",
      "label": "Network",
      "type": "text",
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
      "required": false,
      "default": "connect",
      "options": [
        {
          "value": "connect",
          "label": "Connect"
        },
        {
          "value": "disconnect",
          "label": "Disconnect"
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
