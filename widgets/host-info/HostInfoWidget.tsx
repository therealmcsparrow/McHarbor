// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconServer, IconDatabase, IconClock } from '@tabler/icons-react';
import { StatCard } from '@resources/components/StatCard';
import { splitBytes, formatBytes, formatUptime } from '@resources/utils/format';
import { useHostMetrics } from '@modules/dashboard/hooks/useHostMetrics';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import type { HostMetrics } from '@core/types/docker';

type WidgetConfig = {
  titleKey: string;
  icon: typeof IconCpu;
  getValue: (m: HostMetrics) => string | number;
  getUnit?: (m: HostMetrics, t: (key: string) => string) => string | undefined;
  getDescription?: (m: HostMetrics, t: (key: string, params?: Record<string, string>) => string) => string | undefined;
};

const CONFIG: Record<string, WidgetConfig> = {
  'cpu-cores': {
    titleKey: 'hostInfoWidget.cpuCores',
    icon: IconCpu,
    getValue: (m) => m.host.ncpu,
    getUnit: (_m, t) => t('hostInfoWidget.cores'),
    getDescription: (m) => m.host.architecture,
  },
  'total-memory': {
    titleKey: 'hostInfoWidget.totalMemory',
    icon: IconCpu2,
    getValue: (m) => splitBytes(m.host.memTotal).value,
    getUnit: (m) => splitBytes(m.host.memTotal).unit,
    getDescription: (m) => m.host.os,
  },
  'docker-version': {
    titleKey: 'hostInfoWidget.dockerVersion',
    icon: IconServer,
    getValue: (m) => m.host.serverVersion,
    getDescription: (m) => m.host.hostname,
  },
  'disk-usage': {
    titleKey: 'hostInfoWidget.diskUsage',
    icon: IconDatabase,
    getValue: (m) => splitBytes(m.disk.total).value,
    getUnit: (m) => splitBytes(m.disk.total).unit,
    getDescription: (m, t) =>
      t('hostInfoWidget.imagesVolumes', {
        images: formatBytes(m.disk.imagesSize),
        volumes: formatBytes(m.disk.volumesSize),
      }),
  },
  uptime: {
    titleKey: 'hostInfoWidget.uptime',
    icon: IconClock,
    getValue: (m) => formatUptime(m.host.uptime),
    getDescription: (m) => m.host.hostname,
  },
};

export default function HostInfoWidget({ typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: metrics } = useHostMetrics();
  const cfg = CONFIG[typeId];
  if (!cfg || !metrics) return null;
  const Icon = cfg.icon;

  return (
    <StatCard
      title={t(cfg.titleKey)}
      value={cfg.getValue(metrics)}
      unit={cfg.getUnit?.(metrics, t)}
      description={cfg.getDescription?.(metrics, t)}
      icon={<Icon className="h-5 w-5" />}
      className="h-full border-0"
    />
  );
}
