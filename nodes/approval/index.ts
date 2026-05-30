// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const approval: NodeDefinition = {
  key: 'approval',
  label: 'Approval Gate',
  category: 'logic',
  description: 'Route a workflow through an explicit approved or rejected gate',
  icon: 'IconHandClick',
  configSchema: [
    {
      key: 'decision',
      label: 'Decision',
      type: 'select',
      required: true,
      default: 'pending',
      options: [
        { value: 'approved', label: 'Approved' },
        { value: 'rejected', label: 'Rejected' },
        { value: 'pending', label: 'Pending' },
      ],
    },
    {
      key: 'reason',
      label: 'Reason',
      type: 'textarea',
      required: false,
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['approved', 'rejected', 'pending'],
};
