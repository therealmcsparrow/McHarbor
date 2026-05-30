// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerExec: NodeDefinition = {
  "key": "container-exec",
  "label": "Container Exec",
  "category": "action",
  "description": "Execute a command inside a running container",
  "icon": "IconTerminal",
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
      "key": "command",
      "label": "Command",
      "type": "textarea",
      "required": true
    },
    {
      "key": "workdir",
      "label": "Working Directory",
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
