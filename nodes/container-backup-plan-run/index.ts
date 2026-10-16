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
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    { key: 'container', label: 'Container (filter)', type: 'container-select', required: false },
    { key: 'plan_id', label: 'Backup Plan', type: 'backup-plan-select', required: true },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
