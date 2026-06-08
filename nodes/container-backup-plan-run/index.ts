// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerBackupPlanRun: NodeDefinition = {
  key: 'container-backup-plan-run',
  label: 'Run Backup Plan',
  category: 'action',
  description: 'Run a saved container backup plan',
  icon: 'IconPlayerPlay',
  configSchema: [
    { key: 'plan_id', label: 'Plan ID', type: 'text', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
