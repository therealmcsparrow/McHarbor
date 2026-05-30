// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const nodeScript: NodeDefinition = {
  "key": "node-script",
  "label": "Node Script",
  "category": "utility",
  "description": "Run custom JavaScript against the current workflow message",
  "icon": "IconBrandJavascript",
  "configSchema": [
    {
      "key": "code",
      "label": "Code",
      "type": "code",
      "required": true
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
