// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const itsmStatusTrigger: NodeDefinition = {
  key: 'itsm-status-trigger',
  label: 'Ticket Status',
  category: 'trigger',
  description: 'Start when an ITSM reports that a linked ticket changed state',
  icon: 'IconBellRinging2',
  configSchema: [
    {
      key: 'channel_id',
      label: 'Ticketing Channel',
      type: 'communication-channel-select',
      incidentOnly: true,
      required: false,
    },
    {
      key: 'states',
      label: 'States',
      type: 'multiselect',
      required: false,
      options: [
        { value: 'open', label: 'Open' },
        { value: 'acknowledged', label: 'Acknowledged' },
        { value: 'resolved', label: 'Resolved' },
        { value: 'closed', label: 'Closed' },
        { value: 'cancelled', label: 'Cancelled' },
      ],
    },
  ],
  inputPorts: [],
  outputPorts: ['output'],
};
