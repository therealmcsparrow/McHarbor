// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const imageTag: NodeDefinition = {
  "key": "image-tag",
  "label": "Image Tag",
  "category": "action",
  "description": "Tag an image with a new target reference",
  "icon": "IconTag",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "source",
      "label": "Source Image",
      "type": "text",
      "required": true
    },
    {
      "key": "target",
      "label": "Target Image",
      "type": "text",
      "required": true
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
