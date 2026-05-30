// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerLogs: NodeDefinition = {
  "key": "container-logs",
  "label": "Container Logs",
  "category": "action",
  "description": "Read recent logs from a container",
  "icon": "IconFileText",
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
      "key": "tail",
      "label": "Tail Lines",
      "type": "number",
      "required": false,
      "default": 100
    },
    {
      "key": "since",
      "label": "Since",
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
