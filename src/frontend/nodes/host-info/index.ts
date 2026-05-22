// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const hostInfo: NodeDefinition = {
  "key": "host-info",
  "label": "Host Info",
  "category": "action",
  "description": "Read host information from a Docker environment",
  "icon": "IconDeviceDesktop",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
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
