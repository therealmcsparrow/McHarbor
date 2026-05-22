// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const readFile: NodeDefinition = {
  "key": "read-file",
  "label": "Read File",
  "category": "utility",
  "description": "Read a file from the workflow data directory",
  "icon": "IconFileDownload",
  "configSchema": [
    {
      "key": "path",
      "label": "Path",
      "type": "text",
      "required": true
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
    },
    {
      "key": "output_property",
      "label": "Output Property",
      "type": "expression",
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
