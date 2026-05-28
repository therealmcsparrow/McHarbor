// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconCpu,
  IconDatabase,
  IconDeviceDesktopAnalytics,
  IconInfoCircle,
  IconPackage,
  IconServer,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { StatCard } from '@resources/components/StatCard';
import { splitBytes, formatUptime } from '@resources/utils/format';
import type { BulkContainerMetric } from '@resources/hooks/useContainersBulkStats';
import type { HostMetrics } from '@core/types/docker';
import packageJson from '../../../package.json';
import type { SystemInfo } from '../types';
import { SystemInfoBlock, SystemInfoRow } from './SystemInfoBlock';

export function SystemOverviewTab({
  info,
  hostMetrics,
  containerMetrics,
}: {
  info: SystemInfo;
  hostMetrics?: HostMetrics;
  containerMetrics: BulkContainerMetric[];
}) {
  const { t } = useTranslation('system');
  const frontendVersion = packageJson.version;
  const memory = splitBytes(hostMetrics?.host.memTotal ?? 0);
  const disk = splitBytes(hostMetrics?.disk.total ?? 0);
  const containerMemory = splitBytes(
    containerMetrics.reduce((total, metric) => total + metric.memUsage, 0)
  );
  const containerCpu = containerMetrics.reduce((total, metric) => total + metric.cpuPercent, 0);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          title={t('metrics.cpuCores')}
          value={hostMetrics?.host.ncpu ?? '-'}
          description={hostMetrics?.host.hostname ?? t('metrics.hostPending')}
          icon={<IconCpu className="size-5" />}
        />
        <StatCard
          title={t('metrics.memory')}
          value={memory.value}
          unit={memory.unit}
          description={t('metrics.totalHostMemory')}
          icon={<IconDeviceDesktopAnalytics className="size-5" />}
        />
        <StatCard
          title={t('metrics.dockerDisk')}
          value={disk.value}
          unit={disk.unit}
          description={t('metrics.imagesContainersVolumes')}
          icon={<IconDatabase className="size-5" />}
        />
        <StatCard
          title={t('metrics.containerLoad')}
          value={containerCpu.toFixed(1)}
          unit="%"
          description={t('metrics.containerMemory', {
            value: containerMemory.value,
            unit: containerMemory.unit,
          })}
          icon={<IconServer className="size-5" />}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <SystemInfoBlock
          title={t('overview.application')}
          description={t('overview.applicationDescription')}
          icon={IconInfoCircle}
        >
          <SystemInfoRow label={t('fields.name')} value="McHarbor" />
          <SystemInfoRow label={t('fields.backendVersion')} value={`v${info.version}`} />
          <SystemInfoRow label={t('fields.frontendVersion')} value={`v${frontendVersion}`} />
        </SystemInfoBlock>

        <SystemInfoBlock
          title={t('overview.runtime')}
          description={t('overview.runtimeDescription')}
          icon={IconServer}
        >
          <SystemInfoRow label={t('fields.platform')} value={info.platform} />
          <SystemInfoRow label={t('fields.goVersion')} value={info.goVersion} />
          <SystemInfoRow
            label={t('fields.uptime')}
            value={formatUptime(hostMetrics?.host.uptime ?? 0)}
          />
          <SystemInfoRow
            label={t('fields.status')}
            value={<Badge variant="default">{t('status.available')}</Badge>}
          />
        </SystemInfoBlock>

        <SystemInfoBlock
          title={t('overview.dependencies')}
          description={t('overview.dependenciesDescription')}
          icon={IconPackage}
        >
          <SystemInfoRow
            label={t('fields.backendDependencies')}
            value={info.dependencies.length}
          />
          <SystemInfoRow
            label={t('fields.frontendDependencies')}
            value={Object.keys(packageJson.dependencies ?? {}).length}
          />
          <SystemInfoRow
            label={t('fields.runningProcesses')}
            value={containerMetrics.reduce((total, metric) => total + Number(metric.pids ?? 0), 0)}
          />
        </SystemInfoBlock>
      </div>
    </div>
  );
}
