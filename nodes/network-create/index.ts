// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const networkCreate: NodeDefinition = {
  "key": "network-create",
  "label": "Network Create",
  "category": "action",
  "description": "Create a Docker network",
  "icon": "IconTopologyComplex",
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
      "default": "bridge"
    },
    {
      "key": "internal",
      "label": "Internal",
      "type": "toggle",
      "required": false,
      "default": false
    },
    {
      "key": "subnet",
      "label": "Subnet",
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
