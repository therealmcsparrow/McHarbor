// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const comment: NodeDefinition = {
  "key": "comment",
  "label": "Comment",
  "category": "utility",
  "description": "Annotate a workflow without changing execution",
  "icon": "IconPencil",
  "configSchema": [
    {
      "key": "text",
      "label": "Comment",
      "type": "textarea",
      "required": false
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": [
    "output"
  ]
};
