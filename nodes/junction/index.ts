// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const junction: NodeDefinition = {
  "key": "junction",
  "label": "Junction",
  "category": "utility",
  "description": "Create a simple routing point on the canvas",
  "icon": "IconCircleDot",
  "configSchema": [],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
