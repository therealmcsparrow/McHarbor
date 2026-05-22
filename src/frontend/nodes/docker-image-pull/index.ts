// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dockerImagePull: NodeDefinition = {
  "key": "docker-image-pull",
  "label": "Docker Image Pull",
  "category": "action",
  "description": "Pull an image from a registry",
  "icon": "IconCloudDownload",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "image",
      "label": "Image",
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
