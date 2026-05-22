// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const parseCsv: NodeDefinition = {
  "key": "parse-csv",
  "label": "Parse CSV",
  "category": "utility",
  "description": "Parse CSV or TSV text into structured rows",
  "icon": "IconTable",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "delimiter",
      "label": "Delimiter",
      "type": "text",
      "required": false,
      "default": ","
    },
    {
      "key": "has_headers",
      "label": "Has Headers",
      "type": "toggle",
      "required": false,
      "default": true
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
