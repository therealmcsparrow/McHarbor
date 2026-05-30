// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerCreate: NodeDefinition = {
  "key": "container-create",
  "label": "Container Create",
  "category": "action",
  "description": "Create a new container from an image",
  "icon": "IconSquarePlus",
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
    },
    {
      "key": "name",
      "label": "Name",
      "type": "text",
      "required": false
    },
    {
      "key": "ports",
      "label": "Ports",
      "type": "text",
      "required": false
    },
    {
      "key": "env_vars",
      "label": "Environment Variables",
      "type": "key-value",
      "required": false
    },
    {
      "key": "restart_policy",
      "label": "Restart Policy",
      "type": "select",
      "required": false,
      "default": "no",
      "options": [
        {
          "value": "no",
          "label": "No"
        },
        {
          "value": "always",
          "label": "Always"
        },
        {
          "value": "on-failure",
          "label": "On Failure"
        },
        {
          "value": "unless-stopped",
          "label": "Unless Stopped"
        }
      ]
    },
    {
      "key": "auto_start",
      "label": "Auto Start",
      "type": "toggle",
      "required": false,
      "default": true
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
