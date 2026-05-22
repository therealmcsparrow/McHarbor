// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const composeDown: NodeDefinition = {
  "key": "compose-down",
  "label": "Compose Down",
  "category": "action",
  "description": "Stop and remove a compose stack",
  "icon": "IconPlayerStop",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "stack_name",
      "label": "Stack Name",
      "type": "text",
      "required": true
    },
    {
      "key": "remove_volumes",
      "label": "Remove Volumes",
      "type": "toggle",
      "required": false,
      "default": false
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
