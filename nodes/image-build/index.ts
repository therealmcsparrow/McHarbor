// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const imageBuild: NodeDefinition = {
  "key": "image-build",
  "label": "Image Build",
  "category": "action",
  "description": "Build a Docker image from a workflow-managed build context",
  "icon": "IconHammer",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "tag",
      "label": "Tag",
      "type": "text",
      "required": true
    },
    {
      "key": "dockerfile",
      "label": "Dockerfile",
      "type": "text",
      "required": false,
      "default": "Dockerfile"
    },
    {
      "key": "context_path",
      "label": "Context Path",
      "type": "text",
      "required": false,
      "default": "."
    },
    {
      "key": "target",
      "label": "Target Stage",
      "type": "text",
      "required": false
    },
    {
      "key": "build_args",
      "label": "Build Args",
      "type": "key-value",
      "required": false
    },
    {
      "key": "no_cache",
      "label": "No Cache",
      "type": "toggle",
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
