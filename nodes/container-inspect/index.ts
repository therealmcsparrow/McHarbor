// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerInspect: NodeDefinition = {
  "key": "container-inspect",
  "label": "Container Inspect",
  "category": "action",
  "description": "Inspect a single container",
  "icon": "IconZoomScan",
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
