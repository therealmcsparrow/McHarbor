// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';
import { PERM } from '@core/auth/permissionKeys';

export const itsmResolve: NodeDefinition = {
  key: 'itsm-resolve',
  label: 'Resolve Ticket',
  category: 'integration',
  description: 'Resolve the ticket a key opened',
  icon: 'IconCircleCheck',
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
    { key: 'message', label: 'Message', type: 'textarea', required: false },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
