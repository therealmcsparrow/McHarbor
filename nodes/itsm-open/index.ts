// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';
import { PERM } from '@core/auth/permissionKeys';

export const itsmOpen: NodeDefinition = {
  key: 'itsm-open',
  label: 'Open Ticket',
  category: 'integration',
  description: 'Open or re-notify a ticket in an ITSM through an incident-capable channel',
  icon: 'IconTicket',
  permission: PERM.ALERTS_MANAGE,
  configSchema: [
    {
      key: 'channel_id',
      label: 'Ticketing Channel',
      type: 'communication-channel-select',
      incidentOnly: true,
      required: true,
    },
    { key: 'dedup_key', label: 'Ticket Key', type: 'text', required: true },
    {
      key: 'severity',
      label: 'Severity',
      type: 'select',
      required: false,
      default: 'warning',
      options: [
        { value: 'critical', label: 'Critical' },
        { value: 'warning', label: 'Warning' },
        { value: 'info', label: 'Info' },
      ],
    },
    { key: 'title', label: 'Title', type: 'text', required: true },
    { key: 'message', label: 'Message', type: 'textarea', required: false },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
