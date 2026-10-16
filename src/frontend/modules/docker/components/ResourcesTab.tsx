// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { useHostMetrics } from '@resources/hooks/useHostMetrics';
import type { HostMetrics } from '@core/types/docker';
import { HostHistoryPanel } from './HostHistoryPanel';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-border py-2 last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium text-foreground">{value}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold text-foreground">{title}</h3>
      <div className="rounded-lg border border-border bg-muted/30 px-4">{children}</div>
    </div>
  );
}

export function ResourcesTab() {
  const { data: metrics, isLoading } = useHostMetrics();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  // The live sections need /metrics/host, which fails when the environment's
  // Docker daemon or agent is unreachable. The history panel must not be gated
  // behind that: it reads the stored metric tables and never touches Docker, so
  // it is exactly what is still worth showing when the daemon is down.
  return (
    <div className="space-y-6">
      {metrics && <LiveSections metrics={metrics} />}
      <HostHistoryPanel />
    </div>
  );
}

function LiveSections({ metrics }: { metrics: HostMetrics }) {
  const { t } = useTranslation('docker');
  const { host, disk } = metrics;

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <Section title={t('resources.host')}>
        <InfoRow label={t('resources.hostname')} value={host.hostname} />
        <InfoRow label={t('resources.os')} value={host.os} />
        <InfoRow label={t('resources.architecture')} value={host.architecture} />
        <InfoRow label={t('resources.kernelVersion')} value={host.kernelVersion} />
        <InfoRow label={t('resources.cpus')} value={host.ncpu} />
        <InfoRow label={t('resources.memory')} value={formatBytes(host.memTotal)} />
      </Section>

      <Section title={t('resources.diskUsage')}>
        <InfoRow label={t('resources.imagesSize')} value={formatBytes(disk.imagesSize)} />
        <InfoRow label={t('resources.containersSize')} value={formatBytes(disk.containersSize)} />
        <InfoRow label={t('resources.volumesSize')} value={formatBytes(disk.volumesSize)} />
        <InfoRow label={t('resources.buildCacheSize')} value={formatBytes(disk.buildCacheSize)} />
        <InfoRow label={t('resources.totalDisk')} value={formatBytes(disk.total)} />
      </Section>
    </div>
  );
}
