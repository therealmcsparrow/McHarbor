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
      "key": "registry_mode",
      "label": "Registry",
      "type": "select",
      "required": true,
      "default": "custom",
      "options": [
        {
          "value": "configured",
          "label": "Configured Registry"
        },
        {
          "value": "custom",
          "label": "Other"
        }
      ]
    },
    {
      "key": "registry_id",
      "label": "Registry",
      "type": "registry-select",
      "required": false,
      "showWhen": {
        "registry_mode": "configured"
      }
    },
    {
      "key": "registry_url",
      "label": "Registry URL",
      "type": "text",
      "required": false,
      "showWhen": {
        "registry_mode": "custom"
      }
    },
    {
      "key": "registry_username",
      "label": "Registry Username",
      "type": "text",
      "required": false,
      "showWhen": {
        "registry_mode": "custom"
      }
    },
    {
      "key": "registry_password",
      "label": "Registry Password",
      "type": "text",
      "secret": true,
      "required": false,
      "showWhen": {
        "registry_mode": "custom"
      }
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
