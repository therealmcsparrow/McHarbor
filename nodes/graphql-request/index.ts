// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const graphqlRequest: NodeDefinition = {
  "key": "graphql-request",
  "label": "GraphQL Request",
  "category": "integration",
  "description": "Send a GraphQL request and store the parsed response",
  "icon": "IconApi",
  "configSchema": [
    {
      "key": "url",
      "label": "URL",
      "type": "text",
      "required": true
    },
    {
      "key": "query",
      "label": "Query",
      "type": "textarea",
      "required": true
    },
    {
      "key": "variables",
      "label": "Variables",
      "type": "key-value",
      "required": false
    },
    {
      "key": "headers",
      "label": "Headers",
      "type": "key-value",
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
