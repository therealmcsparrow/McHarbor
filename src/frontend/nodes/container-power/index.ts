// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerPower: NodeDefinition = {
  "key": "container-power",
  "label": "Container Power",
  "category": "action",
  "description": "Enable or disable a container using the power helper action",
  "icon": "IconPlayerPlay",
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
      "key": "action",
      "label": "Action",
      "type": "select",
      "required": true,
      "options": [
        {
          "value": "enable",
          "label": "Enable"
        },
        {
          "value": "disable",
          "label": "Disable"
        }
      ]
    },
    {
      "key": "timeout",
      "label": "Timeout (seconds)",
      "type": "number",
      "required": false,
      "default": 10
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
