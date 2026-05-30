// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const healthCheck: NodeDefinition = {
  "key": "health-check",
  "label": "Health Check",
  "category": "action",
  "description": "Check an HTTP endpoint and branch on health",
  "icon": "IconHeartRateMonitor",
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
      "key": "expected_status",
      "label": "Expected Status",
      "type": "number",
      "required": false,
      "default": 200
    },
    {
      "key": "timeout",
      "label": "Timeout (seconds)",
      "type": "number",
      "required": false,
      "default": 10
    },
    {
      "key": "retries",
      "label": "Retries",
      "type": "number",
      "required": false,
      "default": 3
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "healthy",
    "unhealthy"
  ]
};
