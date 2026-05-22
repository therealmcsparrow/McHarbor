// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendNtfy: NodeDefinition = {
  "key": "send-ntfy",
  "label": "Send ntfy",
  "category": "integration",
  "description": "Publish a message to an ntfy topic",
  "icon": "IconBellPlus",
  "configSchema": [
    {
      "key": "server_url",
      "label": "Server URL",
      "type": "text",
      "required": false,
      "default": "https://ntfy.sh"
    },
    {
      "key": "topic",
      "label": "Topic",
      "type": "text",
      "required": true
    },
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
    },
    {
      "key": "priority",
      "label": "Priority",
      "type": "text",
      "required": false
    },
    {
      "key": "tags",
      "label": "Tags",
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
