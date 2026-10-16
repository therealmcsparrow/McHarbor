// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const sendCommunication: NodeDefinition = {
  key: 'send-communication',
  label: 'Send Message',
  category: 'integration',
  description: 'Send a message through a configured communication channel',
  icon: 'IconSend',
  configSchema: [
    {
      key: 'channel_id',
      label: 'Communication Channel',
      type: 'communication-channel-select',
      required: false,
    },
    { key: 'recipient', label: 'Recipient', type: 'text', required: false },
    { key: 'title', label: 'Title', type: 'text', required: false },
    { key: 'message', label: 'Message', type: 'textarea', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
