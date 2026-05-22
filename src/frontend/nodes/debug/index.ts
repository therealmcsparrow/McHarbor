// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const debug: NodeDefinition = {
  "key": "debug",
  "label": "Debug",
  "category": "utility",
  "description": "Emit the current message to the debug stream",
  "icon": "IconBug",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "full_message",
      "label": "Show Full Message",
      "type": "toggle",
      "required": false,
      "default": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
