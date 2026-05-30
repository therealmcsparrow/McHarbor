// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const httpResponse: NodeDefinition = {
  "key": "http-response",
  "label": "HTTP Response",
  "category": "utility",
  "description": "Set the outgoing status code and headers on the message",
  "icon": "IconApi",
  "configSchema": [
    {
      "key": "status_code",
      "label": "Status Code",
      "type": "number",
      "required": false,
      "default": 200
    },
    {
      "key": "headers",
      "label": "Headers",
      "type": "key-value",
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
