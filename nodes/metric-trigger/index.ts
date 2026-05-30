// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const metricTrigger: NodeDefinition = {
  "key": "metric-trigger",
  "label": "Metric Trigger",
  "category": "trigger",
  "description": "Start a workflow when container metrics are checked",
  "icon": "IconChartLine",
  "configSchema": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "environment-select",
      "required": true
    },
    {
      "key": "container",
      "label": "Container",
      "type": "container-select",
      "required": true
    },
    {
      "key": "conditions",
      "label": "Metric Conditions",
      "type": "metric-conditions",
      "required": false
    },
    {
      "key": "logic",
      "label": "Condition Logic",
      "type": "select",
      "required": false,
      "default": "and",
      "options": [
        {
          "value": "and",
          "label": "AND"
        },
        {
          "value": "or",
          "label": "OR"
        }
      ]
    },
    {
      "key": "interval",
      "label": "Interval (seconds)",
      "type": "number",
      "required": false,
      "default": 10
    },
    {
      "key": "cooldown",
      "label": "Cooldown (seconds)",
      "type": "number",
      "required": false,
      "default": 60
    }
  ],
  "inputPorts": [],
  "outputPorts": [
    "output"
  ]
};
