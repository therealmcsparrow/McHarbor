// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const linkOut: NodeDefinition = {
  "key": "link-out",
  "label": "Link Out",
  "category": "utility",
  "description": "Store the current message for another workflow to consume",
  "icon": "IconArrowBarToLeft",
  "configSchema": [
    {
      "key": "name",
      "label": "Name",
      "type": "text",
      "required": true
    }
  ],
  "inputPorts": [
    "input"
  ],
  "outputPorts": []
};
