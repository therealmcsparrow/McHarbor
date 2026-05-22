// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendWhatsapp: NodeDefinition = {
  "key": "send-whatsapp",
  "label": "Send Whatsapp",
  "category": "integration",
  "description": "Send a WhatsApp message through an API",
  "icon": "IconBrandWhatsapp",
  "configSchema": [
    {
      "key": "api_url",
      "label": "API URL",
      "type": "text",
      "required": true
    },
    {
      "key": "access_token",
      "label": "Access Token",
      "type": "text",
      "secret": true,
      "required": true
    },
    {
      "key": "phone_number",
      "label": "Phone Number",
      "type": "text",
      "required": true
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
