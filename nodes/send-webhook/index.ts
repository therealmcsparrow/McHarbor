// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendWebhook: NodeDefinition = {
  "key": "send-webhook",
  "label": "Send Webhook",
  "category": "integration",
  "description": "Send an outbound webhook with message data",
  "icon": "IconWebhook",
  "configSchema": [
    {
      "key": "delivery_mode",
      "label": "Delivery",
      "type": "select",
      "required": true,
      "default": "custom",
      "options": [
        {
          "value": "configured",
          "label": "Configured Webhook"
        },
        {
          "value": "custom",
          "label": "Other"
        }
      ]
    },
    {
      "key": "webhook_id",
      "label": "Webhook",
      "type": "webhook-select",
      "required": true,
      "showWhen": {
        "delivery_mode": "configured"
      }
    },
    {
      "key": "url",
      "label": "URL",
      "type": "text",
      "required": true,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "method",
      "label": "Method",
      "type": "select",
      "required": false,
      "default": "POST",
      "showWhen": {
        "delivery_mode": "custom"
      },
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
      "key": "headers",
      "label": "Headers",
      "type": "key-value",
      "required": false,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "body_source",
      "label": "Body Source",
      "type": "text",
      "required": false,
      "default": "payload"
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
