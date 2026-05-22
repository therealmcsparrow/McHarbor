// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const fileWatchTrigger: NodeDefinition = {
  "key": "file-watch-trigger",
  "label": "File Watch Trigger",
  "category": "trigger",
  "description": "Start a workflow when a watched file path changes",
  "icon": "IconFileAlert",
  "configSchema": [
    {
      "key": "path",
      "label": "Path",
      "type": "text",
      "required": true
    },
    {
      "key": "event_types",
      "label": "Event Types",
      "type": "text",
      "required": false
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
