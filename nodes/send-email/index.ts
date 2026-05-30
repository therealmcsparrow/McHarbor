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
      "key": "delivery_mode",
      "label": "Delivery",
      "type": "select",
      "required": true,
      "default": "configured",
      "options": [
        {
          "value": "configured",
          "label": "Configured Email Server"
        },
        {
          "value": "custom",
          "label": "Other"
        }
      ]
    },
    {
      "key": "server_id",
      "label": "Email Server",
      "type": "email-server-select",
      "required": false,
      "showWhen": {
        "delivery_mode": "configured"
      }
    },
    {
      "key": "smtp_host",
      "label": "SMTP Host",
      "type": "text",
      "required": true,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "smtp_port",
      "label": "SMTP Port",
      "type": "number",
      "required": true,
      "default": 587,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "smtp_encryption",
      "label": "Encryption",
      "type": "select",
      "required": false,
      "default": "starttls",
      "showWhen": {
        "delivery_mode": "custom"
      },
      "options": [
        {
          "value": "none",
          "label": "None"
        },
        {
          "value": "starttls",
          "label": "STARTTLS"
        },
        {
          "value": "ssl_tls",
          "label": "SSL/TLS"
        }
      ]
    },
    {
      "key": "smtp_auth_method",
      "label": "Auth Method",
      "type": "select",
      "required": false,
      "default": "plain",
      "showWhen": {
        "delivery_mode": "custom"
      },
      "options": [
        {
          "value": "none",
          "label": "None"
        },
        {
          "value": "plain",
          "label": "Plain"
        },
        {
          "value": "login",
          "label": "Login"
        },
        {
          "value": "cram_md5",
          "label": "CRAM-MD5"
        }
      ]
    },
    {
      "key": "smtp_username",
      "label": "Username",
      "type": "text",
      "required": false,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "smtp_password",
      "label": "Password",
      "type": "text",
      "secret": true,
      "required": false,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "from_address",
      "label": "From Address",
      "type": "text",
      "required": true,
      "showWhen": {
        "delivery_mode": "custom"
      }
    },
    {
      "key": "from_name",
      "label": "From Name",
      "type": "text",
      "required": false,
      "showWhen": {
        "delivery_mode": "custom"
      }
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
