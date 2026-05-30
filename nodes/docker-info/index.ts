// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dockerInfo: NodeDefinition = {
  "key": "docker-info",
  "label": "Docker Info",
  "category": "action",
  "description": "Load Docker daemon information from an environment",
  "icon": "IconInfoCircle",
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
