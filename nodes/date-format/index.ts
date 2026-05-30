// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dateFormat: NodeDefinition = {
  "key": "date-format",
  "label": "Date Format",
  "category": "utility",
  "description": "Parse and reformat a date or timestamp",
  "icon": "IconCalendar",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "output_format",
      "label": "Output Format",
      "type": "text",
      "required": false,
      "default": "2006-01-02T15:04:05Z07:00"
    },
    {
      "key": "timezone",
      "label": "Timezone",
      "type": "text",
      "required": false,
      "default": "UTC"
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
