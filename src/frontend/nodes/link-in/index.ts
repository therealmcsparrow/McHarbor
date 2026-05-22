// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const linkIn: NodeDefinition = {
  "key": "link-in",
  "label": "Link In",
  "category": "trigger",
  "description": "Receive the latest message from a link output in another workflow",
  "icon": "IconArrowBarToRight",
  "configSchema": [
    {
      "key": "source",
      "label": "Source Output",
      "type": "link-output-select",
      "required": true
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
