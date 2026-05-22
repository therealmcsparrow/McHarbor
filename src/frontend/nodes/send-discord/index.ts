// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendDiscord: NodeDefinition = {
  "key": "send-discord",
  "label": "Send Discord",
  "category": "integration",
  "description": "Send a message to a Discord webhook",
  "icon": "IconBrandDiscord",
  "configSchema": [
    {
      "key": "webhook_url",
      "label": "Webhook URL",
      "type": "text",
      "secret": true,
      "required": true
    },
    {
      "key": "username",
      "label": "Username",
      "type": "text",
      "required": false
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
