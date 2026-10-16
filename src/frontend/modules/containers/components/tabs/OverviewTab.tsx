// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { formatDate } from '@resources/utils/format';
import { containerStatePresentation } from '@resources/utils/container-state';
import { ContainerStatsPanel } from '../ContainerStatsPanel';
import { ContainerHistoryPanel } from '../ContainerHistoryPanel';
import type { HistoryMetric } from '@resources/components/history-metrics';
import type { ContainerInspect } from '@core/types/docker';
import type { UseContainerStatsResult } from '../../hooks/useContainerStats';
import type { DetailTabId } from '../ContainerDetailTabs';
import { ContainerHealthCard } from '../ContainerHealthCard';
import { ContainerUptimeCard } from '../ContainerUptimeCard';
import { OverviewRuntimeCard } from './OverviewRuntimeCard';
import { OverviewHealthcheckCard } from './OverviewHealthcheckCard';
import { OverviewDevicesLimitsCards } from './OverviewDevicesLimitsCards';
import { useContainerHealthHistory } from '../../hooks/useContainerHealthHistory';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';

const NO_RESTART_POLICY = 'no';

type OverviewTabProps = {
  container: ContainerInspect;
  stats?: UseContainerStatsResult;
  onNavigate: (tab: DetailTabId) => void;
};

export function OverviewTab({ container, stats, onNavigate }: OverviewTabProps) {
  const { t } = useTranslation('containers');
  const state = container.State?.Status ?? 'unknown';
  const containerName = (container.Name ?? '').replace(/^\//, '');
  const isRunning = state === 'running';
  const hc = container.HostConfig;
  const health = container.State?.Health;
  const statePresentation = containerStatePresentation(state, health?.Status);
  const { data: healthHistory, isLoading: healthLoading } = useContainerHealthHistory(container.Id);
  // The four stat tiles drive which series the history panel draws. CPU is the
  // default because it is the first tile and the one people check first.
  const [historyMetric, setHistoryMetric] = useState<HistoryMetric>('cpu');

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <ContainerHealthCard
          history={healthHistory}
          loading={healthLoading}
          // The probe's settings live on Environment, not Resources -
          // Resources holds CPU/memory/restart policy and has nothing about
          // health. Pointing this at Resources sent people looking for health
          // info to a tab that has none.
          onTuneProbe={() => onNavigate('environment')}
        />
        <ContainerUptimeCard history={healthHistory} loading={healthLoading} />
      </div>

      {isRunning && (
        <ContainerStatsPanel
          containerId={container.Id}
          stats={stats}
          activeMetric={historyMetric}
          onShowHistory={setHistoryMetric}
        />
      )}

      {/* Stored history, unlike the live panel above: rendered for stopped
          containers too, since their past is exactly what is worth seeing.
          Those have no tiles to click, so the panel shows its own switcher. */}
      <ContainerHistoryPanel
        containerId={container.Id}
        metric={historyMetric}
        onMetricChange={setHistoryMetric}
        showMetricSwitcher={!isRunning}
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* General Info */}
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('overview.general')}
          </h3>
          <InfoRow label={t('overview.name')}>{containerName}</InfoRow>
          <InfoRow label={t('overview.image')}>{container.Config?.Image ?? '-'}</InfoRow>
          <InfoRow label={t('overview.containerId')}>
            <span className="font-mono text-xs">{container.Id}</span>
          </InfoRow>
          <InfoRow label={t('overview.created')}>{formatDate(container.Created)}</InfoRow>
          <InfoRow label={t('overview.command')}>
            <span className="font-mono text-xs">{container.Config?.Cmd?.join(' ') ?? '-'}</span>
          </InfoRow>
          <InfoRow label={t('overview.entrypoint')}>
            <span className="font-mono text-xs">{container.Config?.Entrypoint?.join(' ') ?? '-'}</span>
          </InfoRow>
          <InfoRow label={t('overview.workingDir')}>
            <span className="font-mono text-xs">{container.Config?.WorkingDir || '/'}</span>
          </InfoRow>
          <InfoRow label={t('overview.restartPolicy')}>
            {hc?.RestartPolicy?.Name ?? NO_RESTART_POLICY}
            {hc?.RestartPolicy?.MaximumRetryCount ? ` (max ${hc.RestartPolicy.MaximumRetryCount})` : ''}
          </InfoRow>
          <InfoRow label={t('overview.platform')}>
            {container.Config?.Domainname
              ? `${container.Config.Hostname}.${container.Config.Domainname}`
              : (container.Config?.Hostname ?? '-')}
          </InfoRow>
          <InfoRow label={t('overview.user')}>
            <span className="font-mono text-xs">{container.Config?.User || '-'}</span>
          </InfoRow>
          <InfoRow label={t('overview.tty')}>
            {container.Config?.Tty ? t('common:labels.yes') : t('common:labels.no')}
            {container.Config?.OpenStdin ? t('overview.interactiveSuffix') : ''}
          </InfoRow>
        </div>

        {/* State Details */}
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('overview.stateDetails')}
          </h3>
          <InfoRow label={t('overview.state')}>
            <StatusIndicator tone={statePresentation.tone} label={state} pulse={state === 'running'} />
          </InfoRow>
          <InfoRow label={t('overview.pid')}>{container.State?.Pid ?? 0}</InfoRow>
          <InfoRow label={t('overview.exitCode')}>{container.State?.ExitCode ?? 0}</InfoRow>
          <InfoRow label={t('overview.oomKilled')}>
            {container.State?.OOMKilled ? (
              <Badge variant="destructive">{t('common:labels.yes')}</Badge>
            ) : (
              t('common:labels.no')
            )}
          </InfoRow>
          {container.State?.Error && (
            <InfoRow label={t('overview.error')}>
              <span className="text-danger">{container.State.Error}</span>
            </InfoRow>
          )}
          <InfoRow label={t('overview.startedAt')}>
            {container.State?.StartedAt ? formatDate(container.State.StartedAt) : '-'}
          </InfoRow>
          <InfoRow label={t('overview.finishedAt')}>
            {container.State?.FinishedAt && container.State.FinishedAt !== '0001-01-01T00:00:00Z'
              ? formatDate(container.State.FinishedAt)
              : '-'}
          </InfoRow>
          <InfoRow label={t('overview.restartCount')}>{container.RestartCount ?? 0}</InfoRow>
          <InfoRow label={t('overview.autoRemove')}>
            {hc?.AutoRemove ? <Badge variant="warning">{t('common:labels.yes')}</Badge> : t('common:labels.no')}
          </InfoRow>
        </div>

        <OverviewRuntimeCard container={container} />

        <OverviewHealthcheckCard container={container} />

        <OverviewDevicesLimitsCards container={container} />
      </div>
    </div>
  );
}
