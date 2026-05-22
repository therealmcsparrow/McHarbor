// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const rateLimit: NodeDefinition = {
  "key": "rate-limit",
  "label": "Rate Limit",
  "category": "logic",
  "description": "Throttle workflow throughput with a simple rate policy",
  "icon": "IconHourglass",
  "configSchema": [
    {
      "key": "limit",
      "label": "Limit",
      "type": "number",
      "required": false,
      "default": 10
    },
    {
      "key": "window_seconds",
      "label": "Window (seconds)",
      "type": "number",
      "required": false,
      "default": 60
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
