// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const httpRequest: NodeDefinition = {
  "key": "http-request",
  "label": "HTTP Request",
  "category": "action",
  "description": "Send an HTTP request and store the response on the message",
  "icon": "IconWorld",
  "configSchema": [
    {
      "key": "url",
      "label": "URL",
      "type": "text",
      "required": true
    },
    {
      "key": "method",
      "label": "Method",
      "type": "select",
      "required": false,
      "default": "GET",
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
      "required": false
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
