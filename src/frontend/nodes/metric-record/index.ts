// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const metricRecord: NodeDefinition = {
  "key": "metric-record",
  "label": "Metric Record",
  "category": "utility",
  "description": "Persist a workflow metric sample from the current message",
  "icon": "IconChartDots",
  "configSchema": [
    {
      "key": "metric_name",
      "label": "Metric Name",
      "type": "text",
      "required": true
    },
    {
      "key": "property",
      "label": "Property",
      "type": "expression",
      "required": false,
      "default": "payload"
    },
    {
      "key": "metric_type",
      "label": "Metric Type",
      "type": "select",
      "required": false,
      "default": "gauge",
      "options": [
        {
          "value": "gauge",
          "label": "Gauge"
        },
        {
          "value": "counter",
          "label": "Counter"
        },
        {
          "value": "event",
          "label": "Event"
        }
      ]
    },
    {
      "key": "unit",
      "label": "Unit",
      "type": "text",
      "required": false
    },
    {
      "key": "labels",
      "label": "Labels",
      "type": "key-value",
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
