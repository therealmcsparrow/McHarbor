// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { DemoConfigGroup, DemoFieldProblem } from '@resources/constants/demoConfig';
import type { DemoConfigField } from '@resources/types/demo';

export type DemoConfigLabels = {
  field: Record<DemoConfigField, string>;
  group: Record<DemoConfigGroup, string>;
  problem: (problem: DemoFieldProblem) => string;
};

export function useDemoConfigLabels(): DemoConfigLabels {
  const { t, i18n } = useTranslation('demo');
  const format = (value: number) => new Intl.NumberFormat(i18n.language).format(value);
  return {
    field: {
      environments: t('estate.environments'),
      containers: t('estate.containers'),
      virtualMachines: t('estate.virtualMachines'),
      systemContainers: t('estate.systemContainers'),
      kubernetesClusters: t('estate.clusters'),
      kubernetesDeployments: t('estate.deployments'),
      swarmClusters: t('estate.swarmClusters'),
      swarmNodes: t('estate.swarmNodes'),
      swarmServices: t('estate.swarmServices'),
      historyDays: t('estate.historyDays'),
    },
    group: {
      estate: t('config.groups.estate'),
      compute: t('config.groups.compute'),
      kubernetes: t('config.groups.kubernetes'),
      swarm: t('config.groups.swarm'),
      history: t('config.groups.history'),
    },
    problem: (problem) =>
      problem.kind === 'swarmNodes'
        ? t('config.error.swarmNodes', { min: format(problem.min) })
        : t('config.error.range', { min: format(problem.min), max: format(problem.max) }),
  };
}
