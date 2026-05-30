// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerHealthTrigger: NodeDefinition = {
  key: 'container-health-trigger',
  label: 'Container Health Trigger',
  category: 'trigger',
  description: 'Start a workflow when a container health status changes',
  icon: 'IconHeartRateMonitor',
  configSchema: [
    {
      key: 'environment',
      label: 'Environment',
      type: 'environment-select',
      required: true,
    },
    {
      key: 'container',
      label: 'Container',
      type: 'container-select',
      required: true,
    },
    {
      key: 'health',
      label: 'Health Status',
      type: 'select',
      required: false,
      default: 'any',
      options: [
        { value: 'any', label: 'Any' },
        { value: 'healthy', label: 'Healthy' },
        { value: 'unhealthy', label: 'Unhealthy' },
        { value: 'starting', label: 'Starting' },
        { value: 'none', label: 'No Healthcheck' },
      ],
    },
  ],
  inputPorts: [],
  outputPorts: ['output'],
};
