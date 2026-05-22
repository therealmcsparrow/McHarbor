// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const deduplicate: NodeDefinition = {
  "key": "deduplicate",
  "label": "Deduplicate",
  "category": "logic",
  "description": "Remove duplicate items from an array",
  "icon": "IconCopyOff",
  "configSchema": [
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "unique_by",
      "label": "Unique By",
      "type": "text",
      "required": false
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
