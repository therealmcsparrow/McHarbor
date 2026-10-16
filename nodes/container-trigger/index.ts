// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeDefinition } from '@modules/workflows/types';

export const containerTrigger: NodeDefinition = {
  key: 'container-trigger',
  label: 'Container Trigger',
  category: 'trigger',
  description: 'Start a workflow when a container changes state or health',
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
      key: 'watch',
      label: 'Watch',
      type: 'select',
      required: true,
      default: 'status',
      options: [
        {
          value: 'status',
          label: 'State changes',
        },
        {
          value: 'health',
          label: 'Health changes',
        },
      ],
    },
    {
      key: 'status',
      label: 'Status Event',
      type: 'select',
      required: false,
      default: 'any',
      options: [
        {
          value: 'any',
          label: 'Any',
        },
        {
          value: 'start',
          label: 'Start',
        },
        {
          value: 'stop',
          label: 'Stop',
        },
        {
          value: 'die',
          label: 'Die',
        },
        {
          value: 'health_status',
          label: 'Health Status',
        },
      ],
      showWhen: {
        watch: 'status',
      },
    },
    {
      key: 'health',
      label: 'Health Status',
      type: 'select',
      required: false,
      default: 'any',
      options: [
        {
          value: 'any',
          label: 'Any',
        },
        {
          value: 'healthy',
          label: 'Healthy',
        },
        {
          value: 'unhealthy',
          label: 'Unhealthy',
        },
        {
          value: 'starting',
          label: 'Starting',
        },
        {
          value: 'none',
          label: 'No Healthcheck',
        },
      ],
      showWhen: {
        watch: 'health',
      },
    },
  ],
  inputPorts: [],
  outputPorts: ['output'],
};
