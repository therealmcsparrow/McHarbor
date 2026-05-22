// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendSignal: NodeDefinition = {
  "key": "send-signal",
  "label": "Send Signal",
  "category": "integration",
  "description": "Send a message through a Signal API bridge",
  "icon": "IconMessageCircle",
  "configSchema": [
    {
      "key": "api_url",
      "label": "API URL",
      "type": "text",
      "required": true
    },
    {
      "key": "sender",
      "label": "Sender",
      "type": "text",
      "required": true
    },
    {
      "key": "recipient",
      "label": "Recipient",
      "type": "text",
      "required": true
    },
    {
      "key": "message",
      "label": "Message",
      "type": "textarea",
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
