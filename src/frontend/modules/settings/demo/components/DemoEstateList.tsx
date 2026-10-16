// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { DemoConfig, DemoCounts } from '@resources/types/demo';

type EstateRow = { key: string; label: string; value: string };

type DemoEstateListProps = {
  counts?: DemoCounts;
  config: DemoConfig;
};

export function DemoEstateList({ counts, config }: DemoEstateListProps) {
  const { t, i18n } = useTranslation('demo');
  const format = (value: number) => new Intl.NumberFormat(i18n.language).format(value);
  const days = (value: number) => t('estate.days', { count: value, value: format(value) });

  const live = (source: DemoCounts): EstateRow[] => [
    { key: 'environments', label: t('estate.environments'), value: format(source.environments) },
    { key: 'containers', label: t('estate.containers'), value: format(source.containers) },
    { key: 'stacks', label: t('estate.stacks'), value: format(source.stacks) },
    { key: 'virtualMachines', label: t('estate.virtualMachines'), value: format(source.virtualMachines) },
    { key: 'systemContainers', label: t('estate.systemContainers'), value: format(source.systemContainers) },
    { key: 'clusters', label: t('estate.clusters'), value: format(source.clusters) },
    { key: 'pods', label: t('estate.pods'), value: format(source.pods) },
    { key: 'swarmClusters', label: t('estate.swarmClusters'), value: format(source.swarmClusters) },
    { key: 'swarmNodes', label: t('estate.swarmNodes'), value: format(source.swarmNodes) },
    { key: 'swarmServices', label: t('estate.swarmServices'), value: format(source.swarmServices) },
    { key: 'historyDays', label: t('estate.historyDays'), value: days(source.historyDays) },
  ];

  const configured = (source: DemoConfig): EstateRow[] => [
    { key: 'environments', label: t('estate.environments'), value: format(source.environments) },
    { key: 'containers', label: t('estate.containers'), value: format(source.containers) },
    { key: 'virtualMachines', label: t('estate.virtualMachines'), value: format(source.virtualMachines) },
    { key: 'systemContainers', label: t('estate.systemContainers'), value: format(source.systemContainers) },
    { key: 'kubernetesClusters', label: t('estate.clusters'), value: format(source.kubernetesClusters) },
    { key: 'kubernetesDeployments', label: t('estate.deployments'), value: format(source.kubernetesDeployments) },
    { key: 'swarmClusters', label: t('estate.swarmClusters'), value: format(source.swarmClusters) },
    { key: 'swarmNodes', label: t('estate.swarmNodes'), value: format(source.swarmNodes) },
    { key: 'swarmServices', label: t('estate.swarmServices'), value: format(source.swarmServices) },
    { key: 'historyDays', label: t('estate.historyDays'), value: days(source.historyDays) },
  ];

  const rows = counts ? live(counts) : configured(config);

  return (
    <Stack gap={2}>
      <TypographyText weight="medium">{counts ? t('estate.liveTitle') : t('estate.title')}</TypographyText>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
        {rows.map((row) => (
          <div key={row.key} className="rounded-lg border border-border bg-muted/20 p-3">
            <dt className="text-xs text-muted-foreground">{row.label}</dt>
            <dd className="mt-1 text-lg font-semibold text-foreground tabular-nums">{row.value}</dd>
          </div>
        ))}
      </dl>
    </Stack>
  );
}
