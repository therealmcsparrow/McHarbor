// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const log: NodeDefinition = {
  "key": "log",
  "label": "Log",
  "category": "utility",
  "description": "Write a message to the workflow log stream",
  "icon": "IconFileText",
  "configSchema": [
    {
      "key": "level",
      "label": "Level",
      "type": "select",
      "required": false,
      "default": "info",
      "options": [
        {
          "value": "info",
          "label": "Info"
        },
        {
          "value": "warn",
          "label": "Warn"
        },
        {
          "value": "error",
          "label": "Error"
        }
      ]
    },
    {
      "key": "message",
      "label": "Message",
      "type": "textarea",
      "required": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
