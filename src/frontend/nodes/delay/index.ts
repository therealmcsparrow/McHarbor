// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const delay: NodeDefinition = {
  "key": "delay",
  "label": "Delay",
  "category": "utility",
  "description": "Pause workflow execution for a number of seconds",
  "icon": "IconClockPause",
  "configSchema": [
    {
      "key": "seconds",
      "label": "Seconds",
      "type": "number",
      "required": false,
      "default": 1
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
