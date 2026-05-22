// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const cronTrigger: NodeDefinition = {
  "key": "cron-trigger",
  "label": "Cron Trigger",
  "category": "trigger",
  "description": "Emit a message from a cron expression and timezone",
  "icon": "IconCalendarClock",
  "configSchema": [
    {
      "key": "cron",
      "label": "Cron Expression",
      "type": "cron",
      "required": true
    },
    {
      "key": "timezone",
      "label": "Timezone",
      "type": "text",
      "required": false,
      "default": "UTC"
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
