// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const template: NodeDefinition = {
  "key": "template",
  "label": "Template",
  "category": "utility",
  "description": "Render a text template using values from the workflow message",
  "icon": "IconTemplate",
  "configSchema": [
    {
      "key": "template",
      "label": "Template",
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
