// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const webhookResponse: NodeDefinition = {
  "key": "webhook-response",
  "label": "Webhook Response",
  "category": "utility",
  "description": "Prepare an HTTP response for a webhook-triggered flow",
  "icon": "IconArrowBackUp",
  "configSchema": [
    {
      "key": "status_code",
      "label": "Status Code",
      "type": "number",
      "required": false,
      "default": 200
    },
    {
      "key": "content_type",
      "label": "Content Type",
      "type": "text",
      "required": false,
      "default": "application/json"
    },
    {
      "key": "body",
      "label": "Body",
      "type": "textarea",
      "required": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
