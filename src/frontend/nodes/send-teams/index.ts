// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendTeams: NodeDefinition = {
  "key": "send-teams",
  "label": "Send Teams",
  "category": "integration",
  "description": "Send a message to a Microsoft Teams webhook",
  "icon": "IconBrandTeams",
  "configSchema": [
    {
      "key": "webhook_url",
      "label": "Webhook URL",
      "type": "text",
      "secret": true,
      "required": true
    },
    {
      "key": "title",
      "label": "Title",
      "type": "text",
      "required": false
    },
    {
      "key": "color",
      "label": "Color",
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
