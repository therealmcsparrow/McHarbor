// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const ping: NodeDefinition = {
  "key": "ping",
  "label": "Ping",
  "category": "integration",
  "description": "Check whether a host is reachable over TCP",
  "icon": "IconWifi",
  "configSchema": [
    {
      "key": "host",
      "label": "Host",
      "type": "text",
      "required": true
    },
    {
      "key": "timeout",
      "label": "Timeout (seconds)",
      "type": "number",
      "required": false,
      "default": 5
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
