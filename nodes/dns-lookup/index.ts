// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dnsLookup: NodeDefinition = {
  "key": "dns-lookup",
  "label": "DNS Lookup",
  "category": "integration",
  "description": "Resolve DNS records for a hostname",
  "icon": "IconWorldSearch",
  "configSchema": [
    {
      "key": "hostname",
      "label": "Hostname",
      "type": "text",
      "required": true
    },
    {
      "key": "record_type",
      "label": "Record Type",
      "type": "select",
      "required": false,
      "default": "A",
      "options": [
        {
          "value": "A",
          "label": "A"
        },
        {
          "value": "AAAA",
          "label": "AAAA"
        },
        {
          "value": "MX",
          "label": "MX"
        },
        {
          "value": "TXT",
          "label": "TXT"
        },
        {
          "value": "CNAME",
          "label": "CNAME"
        },
        {
          "value": "NS",
          "label": "NS"
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
