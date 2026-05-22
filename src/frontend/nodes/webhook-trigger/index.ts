// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const webhookTrigger: NodeDefinition = {
  "key": "webhook-trigger",
  "label": "Webhook Trigger",
  "category": "trigger",
  "description": "Start a workflow from an incoming webhook request",
  "icon": "IconWebhook",
  "configSchema": [
    {
      "key": "method",
      "label": "Method",
      "type": "select",
      "required": false,
      "default": "POST",
      "options": [
        {
          "value": "GET",
          "label": "GET"
        },
        {
          "value": "POST",
          "label": "POST"
        },
        {
          "value": "PUT",
          "label": "PUT"
        },
        {
          "value": "PATCH",
          "label": "PATCH"
        },
        {
          "value": "DELETE",
          "label": "DELETE"
        }
      ]
    },
    {
      "key": "path",
      "label": "Path",
      "type": "text",
      "required": false
    },
    {
      "key": "secret",
      "label": "Secret",
      "type": "text",
      "secret": true,
      "required": false
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
