// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerWait: NodeDefinition = {
  "key": "container-wait",
  "label": "Container Wait",
  "category": "action",
  "description": "Wait for a container to reach a condition",
  "icon": "IconHourglass",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "container",
      "label": "Container",
      "type": "container-select",
      "required": true
    },
    {
      "key": "condition",
      "label": "Condition",
      "type": "select",
      "required": false,
      "default": "not-running",
      "options": [
        {
          "value": "not-running",
          "label": "Not Running"
        },
        {
          "value": "next-exit",
          "label": "Next Exit"
        },
        {
          "value": "removed",
          "label": "Removed"
        }
      ]
    },
    {
      "key": "timeout",
      "label": "Timeout (seconds)",
      "type": "number",
      "required": false,
      "default": 60
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
