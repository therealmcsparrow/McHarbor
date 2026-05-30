// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const ftpUpload: NodeDefinition = {
  "key": "ftp-upload",
  "label": "FTP Upload",
  "category": "integration",
  "description": "Upload workflow data or a saved file with FTP or SFTP",
  "icon": "IconUpload",
  "configSchema": [
    {
      "key": "host",
      "label": "Host",
      "type": "text",
      "required": true
    },
    {
      "key": "port",
      "label": "Port",
      "type": "number",
      "required": false
    },
    {
      "key": "username",
      "label": "Username",
      "type": "text",
      "required": false
    },
    {
      "key": "password",
      "label": "Password",
      "type": "text",
      "secret": true,
      "required": false
    },
    {
      "key": "protocol",
      "label": "Protocol",
      "type": "select",
      "required": false,
      "default": "ftp",
      "options": [
        {
          "value": "ftp",
          "label": "FTP"
        },
        {
          "value": "sftp",
          "label": "SFTP"
        }
      ]
    },
    {
      "key": "remote_path",
      "label": "Remote Path",
      "type": "text",
      "required": true
    },
    {
      "key": "source_mode",
      "label": "Source Mode",
      "type": "select",
      "required": false,
      "default": "payload",
      "options": [
        {
          "value": "payload",
          "label": "Message Property"
        },
        {
          "value": "file",
          "label": "File Path"
        }
      ]
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload",
      "showWhen": {
        "source_mode": "payload"
      }
    },
    {
      "key": "local_path",
      "label": "Local Path",
      "type": "text",
      "required": false,
      "showWhen": {
        "source_mode": "file"
      }
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
