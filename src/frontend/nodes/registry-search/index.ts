// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const registrySearch: NodeDefinition = {
  "key": "registry-search",
  "label": "Registry Search",
  "category": "action",
  "description": "Search a container registry for images",
  "icon": "IconWorldSearch",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "term",
      "label": "Search Term",
      "type": "text",
      "required": true
    },
    {
      "key": "limit",
      "label": "Limit",
      "type": "number",
      "required": false,
      "default": 25
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
