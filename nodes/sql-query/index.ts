// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sqlQuery: NodeDefinition = {
  "key": "sql-query",
  "label": "SQL Query",
  "category": "utility",
  "description": "Run a read-only SQL query against the workflow database",
  "icon": "IconSql",
  "configSchema": [
    {
      "key": "query",
      "label": "Query",
      "type": "textarea",
      "required": true
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
