// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const scheduleTrigger: NodeDefinition = {
  "key": "schedule-trigger",
  "label": "Schedule Trigger",
  "category": "trigger",
  "description": "Run a workflow on a cron-based schedule",
  "icon": "IconClock",
  "configSchema": [
    {
      "key": "cron",
      "label": "Cron Expression",
      "type": "cron",
      "required": true
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
