// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerStatusTrigger: NodeDefinition = {
  "key": "container-status-trigger",
  "label": "Container Status Trigger",
  "category": "trigger",
  "description": "Start a workflow when a container changes state",
  "icon": "IconBox",
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
      "key": "status",
      "label": "Status Event",
      "type": "select",
      "required": false,
      "default": "any",
      "options": [
        {
          "value": "any",
          "label": "Any"
        },
        {
          "value": "start",
          "label": "Start"
        },
        {
          "value": "stop",
          "label": "Stop"
        },
        {
          "value": "die",
          "label": "Die"
        },
        {
          "value": "health_status",
          "label": "Health Status"
        }
      ]
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
