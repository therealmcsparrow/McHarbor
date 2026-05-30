// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stackStatus: NodeDefinition = {
  "key": "stack-status",
  "label": "Stack Status",
  "category": "action",
  "description": "Inspect the status of a compose stack",
  "icon": "IconHeartbeat",
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
