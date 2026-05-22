// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const writeFile: NodeDefinition = {
  "key": "write-file",
  "label": "Write File",
  "category": "utility",
  "description": "Write message data to a file in the workflow data directory",
  "icon": "IconFileUpload",
  "configSchema": [
    {
      "key": "path",
      "label": "Path",
      "type": "text",
      "required": true
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "mode",
      "label": "Mode",
      "type": "select",
      "required": false,
      "default": "overwrite",
      "options": [
        {
          "value": "overwrite",
          "label": "Overwrite"
        },
        {
          "value": "append",
          "label": "Append"
        }
      ]
    },
    {
      "key": "encoding",
      "label": "Encoding",
      "type": "select",
      "required": false,
      "default": "utf-8",
      "options": [
        {
          "value": "utf-8",
          "label": "UTF-8"
        },
        {
          "value": "base64",
          "label": "Base64"
        }
      ]
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
