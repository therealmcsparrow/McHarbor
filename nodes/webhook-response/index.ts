// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const webhookResponse: NodeDefinition = {
  key: 'webhook-response',
  label: 'Webhook Response',
  category: 'utility',
  description: 'Prepare an HTTP response for a webhook-triggered flow',
  icon: 'IconArrowBackUp',
  configSchema: [
    {
      key: 'status_code',
      label: 'Status Code',
      type: 'number',
      required: false,
      default: 200,
    },
    {
      key: 'content_type',
      label: 'Content Type',
      type: 'select',
      allowCustom: true,
      options: [
        {
          value: 'application/json',
          label: 'JSON',
        },
        {
          value: 'text/plain',
          label: 'Plain text',
        },
        {
          value: 'text/html',
          label: 'HTML',
        },
        {
          value: 'application/xml',
          label: 'XML',
        },
        {
          value: 'application/x-www-form-urlencoded',
          label: 'Form (URL-encoded)',
        },
        {
          value: 'application/octet-stream',
          label: 'Binary (octet-stream)',
        },
      ],
      required: false,
      default: 'application/json',
    },
    {
      key: 'body',
      label: 'Body',
      type: 'textarea',
      required: false,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output'],
};
