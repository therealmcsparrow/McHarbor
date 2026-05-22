// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const hostExec: NodeDefinition = {
  "key": "host-exec",
  "label": "Host Exec",
  "category": "action",
  "description": "Run a host command through a temporary container",
  "icon": "IconTerminal2",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "command",
      "label": "Command",
      "type": "textarea",
      "required": true
    },
    {
      "key": "timeout",
      "label": "Timeout (seconds)",
      "type": "number",
      "required": false,
      "default": 30
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
