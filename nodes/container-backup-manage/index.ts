// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerBackupManage: NodeDefinition = {
  key: 'container-backup-manage',
  label: 'Container Backup Manage',
  category: 'action',
  description: 'List backup plans or runs, or get, cancel and delete a backup run',
  icon: 'IconList',
  configSchema: [
    { key: 'environment', label: 'Environment', type: 'environment-select', required: true },
    {
      key: 'action',
      label: 'Action',
      type: 'select',
      required: true,
      default: 'list_runs',
      options: [
        { value: 'list_plans', label: 'List plans' },
        { value: 'list_runs', label: 'List runs' },
        { value: 'get_run', label: 'Get run' },
        { value: 'cancel_run', label: 'Cancel run' },
        { value: 'delete_run', label: 'Delete run' },
      ],
    },
    {
      key: 'container',
      label: 'Container (filter)',
      type: 'container-select',
      required: false,
      showWhen: { action: ['list_plans', 'list_runs'] },
    },
    {
      key: 'status',
      label: 'Status',
      type: 'select',
      required: false,
      default: 'all',
      options: [
        { value: 'all', label: 'All' },
        { value: 'completed', label: 'Completed' },
        { value: 'running', label: 'Running' },
        { value: 'failed', label: 'Failed' },
        { value: 'cancelled', label: 'Cancelled' },
      ],
      showWhen: { action: 'list_runs' },
    },
    { key: 'limit', label: 'Limit', type: 'number', required: false, default: 25, showWhen: { action: 'list_runs' } },
    {
      key: 'run_id',
      label: 'Backup Run',
      type: 'backup-run-select',
      required: false,
      showWhen: { action: ['get_run', 'cancel_run', 'delete_run'] },
    },
  ],
  inputPorts: ['input'],
  outputPorts: ['output', 'error'],
};
