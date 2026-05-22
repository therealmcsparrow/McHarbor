// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendEmail: NodeDefinition = {
  "key": "send-email",
  "label": "Send Email",
  "category": "integration",
  "description": "Send an email through a configured email server",
  "icon": "IconMail",
  "configSchema": [
    {
      "key": "server_id",
      "label": "Email Server",
      "type": "email-server-select",
      "required": false
    },
    {
      "key": "to",
      "label": "To",
      "type": "text",
      "required": true
    },
    {
      "key": "subject",
      "label": "Subject",
      "type": "text",
      "required": false
    },
    {
      "key": "body",
      "label": "Body",
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
