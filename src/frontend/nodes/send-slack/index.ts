// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendSlack: NodeDefinition = {
  "key": "send-slack",
  "label": "Send Slack",
  "category": "integration",
  "description": "Send a message to a Slack webhook",
  "icon": "IconBrandSlack",
  "configSchema": [
    {
      "key": "webhook_url",
      "label": "Webhook URL",
      "type": "text",
      "secret": true,
      "required": true
    },
    {
      "key": "channel",
      "label": "Channel",
      "type": "text",
      "required": false
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
