// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const parseHtml: NodeDefinition = {
  "key": "parse-html",
  "label": "Parse HTML",
  "category": "utility",
  "description": "Extract text, HTML, or attributes from HTML content",
  "icon": "IconHtml",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "selector",
      "label": "Selector",
      "type": "text",
      "required": false,
      "default": "body"
    },
    {
      "key": "output_type",
      "label": "Output Type",
      "type": "select",
      "required": false,
      "default": "text",
      "options": [
        {
          "value": "text",
          "label": "Text"
        },
        {
          "value": "html",
          "label": "HTML"
        },
        {
          "value": "attribute",
          "label": "Attribute"
        }
      ]
    },
    {
      "key": "attribute",
      "label": "Attribute",
      "type": "text",
      "required": false,
      "showWhen": {
        "output_type": "attribute"
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
