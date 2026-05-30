// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendNotification: NodeDefinition = {
  "key": "send-notification",
  "label": "Send Internal Notification",
  "category": "integration",
  "description": "Send an in-app notification inside McHarbor",
  "icon": "IconBell",
  "configSchema": [
    {
      "key": "title",
      "label": "Title",
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
