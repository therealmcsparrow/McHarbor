// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const stackDeploy: NodeDefinition = {
  "key": "stack-deploy",
  "label": "Stack Deploy",
  "category": "action",
  "description": "Deploy a Docker Compose stack from workflow data, inline YAML, or a saved file",
  "icon": "IconRocket",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "stack_name",
      "label": "Stack Name",
      "type": "text",
      "required": true
    },
    {
      "key": "compose_source",
      "label": "Compose Source",
      "type": "select",
      "required": false,
      "default": "message",
      "options": [
        {
          "value": "message",
          "label": "Message"
        },
        {
          "value": "inline",
          "label": "Inline YAML"
        },
        {
          "value": "file",
          "label": "File Path"
        }
      ]
    },
    {
      "key": "compose_content",
      "label": "Compose Content",
      "type": "textarea",
      "required": false,
      "showWhen": {
        "compose_source": "inline"
      }
    },
    {
      "key": "compose_path",
      "label": "Compose Path",
      "type": "text",
      "required": false,
      "showWhen": {
        "compose_source": "file"
      }
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
