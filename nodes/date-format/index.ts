// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const dateFormat: NodeDefinition = {
  key: 'date-format',
  label: 'Date Format',
  category: 'utility',
  description: 'Parse and reformat a date or timestamp',
  icon: 'IconCalendar',
  configSchema: [
    {
      key: 'property',
      label: 'Property',
      type: 'expression',
      required: false,
      default: 'payload',
    },
    {
      key: 'output_format',
      label: 'Output Format',
      type: 'select',
      allowCustom: true,
      options: [
        {
          value: 'rfc3339',
          label: 'ISO 8601 / RFC 3339',
        },
        {
          value: 'rfc1123',
          label: 'RFC 1123',
        },
        {
          value: 'date',
          label: 'Date (YYYY-MM-DD)',
        },
        {
          value: 'datetime',
          label: 'Date and time (YYYY-MM-DD HH:MM:SS)',
        },
        {
          value: 'time',
          label: 'Time (HH:MM:SS)',
        },
        {
          value: 'date_eu',
          label: 'Date (DD-MM-YYYY)',
        },
        {
          value: 'date_us',
          label: 'Date (MM/DD/YYYY)',
        },
        {
          value: 'unix',
          label: 'Unix timestamp',
        },
      ],
      required: false,
      default: 'rfc3339',
    },
    {
      key: 'timezone',
      label: 'Timezone',
      type: 'timezone-select',
      required: false,
      default: 'UTC',
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
