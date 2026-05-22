// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { formatDate } from '@resources/utils/format';
import { ContainerStatsPanel } from '../ContainerStatsPanel';
import type { ContainerInspect } from '@core/types/docker';

const STATE_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  running: 'success',
  exited: 'destructive',
  paused: 'warning',
  restarting: 'warning',
  created: 'secondary',
  dead: 'destructive',
};

const HEALTH_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  healthy: 'success',
  unhealthy: 'destructive',
  starting: 'warning',
};

type OverviewTabProps = {
  container: ContainerInspect;
};

export function OverviewTab({ container }: OverviewTabProps) {
  const { t } = useTranslation('containers');
  const state = container.State?.Status ?? 'unknown';
  const name = (container.Name ?? '').replace(/^\//, '');
  const isRunning = state === 'running';
  const hc = container.HostConfig;
  const health = container.State?.Health;
  const healthcheck = container.Config?.Healthcheck;

  return (
    <div className="space-y-6">
      {isRunning && <ContainerStatsPanel containerId={container.Id} />}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* General Info */}
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.general')}</h3>
          <InfoRow label={t('overview.name')}>{name}</InfoRow>
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
            {hc?.RestartPolicy?.Name ?? 'no'}
            {hc?.RestartPolicy?.MaximumRetryCount
              ? ` (max ${hc.RestartPolicy.MaximumRetryCount})`
              : ''}
          </InfoRow>
          <InfoRow label={t('overview.platform')}>
            {container.Config?.Domainname ? `${container.Config.Hostname}.${container.Config.Domainname}` : container.Config?.Hostname ?? '-'}
          </InfoRow>
          <InfoRow label={t('overview.user')}>
            <span className="font-mono text-xs">{container.Config?.User || '-'}</span>
          </InfoRow>
          <InfoRow label={t('overview.tty')}>
            {container.Config?.Tty ? t('common:labels.yes') : t('common:labels.no')}
            {container.Config?.OpenStdin ? ' + Interactive' : ''}
          </InfoRow>
        </div>

        {/* State Details */}
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.stateDetails')}</h3>
          <InfoRow label={t('overview.state')}>
            <Badge variant={STATE_VARIANTS[state] ?? 'secondary'}>{state}</Badge>
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
              <span className="text-red-500">{container.State.Error}</span>
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

        {/* Runtime */}
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.runtime')}</h3>
          <InfoRow label={t('overview.stopSignal')}>{container.Config?.StopSignal ?? 'SIGTERM'}</InfoRow>
          <InfoRow label={t('overview.runtimeEngine')}>{hc?.Runtime || 'runc'}</InfoRow>
          <InfoRow label={t('overview.logDriver')}>{hc?.LogConfig?.Type ?? 'json-file'}</InfoRow>
          {hc?.LogConfig?.Config && Object.keys(hc.LogConfig.Config).length > 0 && (
            <InfoRow label={t('overview.logOptions')}>
              <div className="space-y-0.5">
                {Object.entries(hc.LogConfig.Config).map(([k, v]) => (
                  <div key={k} className="font-mono text-xs">
                    <span className="text-muted-foreground">{k}=</span>{v}
                  </div>
                ))}
              </div>
            </InfoRow>
          )}
          <InfoRow label={t('overview.init')}>
            {hc?.Init ? t('common:labels.yes') : t('common:labels.no')}
          </InfoRow>
          {hc?.PidsLimit !== null && hc?.PidsLimit !== undefined && hc.PidsLimit > 0 && (
            <InfoRow label={t('overview.pidsLimit')}>{hc.PidsLimit}</InfoRow>
          )}
        </div>

        {/* Health Check */}
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.healthcheck')}</h3>
          {healthcheck?.Test && healthcheck.Test.length > 0 && healthcheck.Test[0] !== 'NONE' ? (
            <>
              <InfoRow label={t('overview.healthCommand')}>
                <span className="font-mono text-xs">{healthcheck.Test.slice(1).join(' ')}</span>
              </InfoRow>
              <InfoRow label={t('overview.healthInterval')}>
                {healthcheck.Interval ? `${healthcheck.Interval / 1e9}s` : '-'}
              </InfoRow>
              <InfoRow label={t('overview.healthTimeout')}>
                {healthcheck.Timeout ? `${healthcheck.Timeout / 1e9}s` : '-'}
              </InfoRow>
              <InfoRow label={t('overview.healthRetries')}>{healthcheck.Retries ?? '-'}</InfoRow>
              <InfoRow label={t('overview.healthStartPeriod')}>
                {healthcheck.StartPeriod ? `${healthcheck.StartPeriod / 1e9}s` : '-'}
              </InfoRow>
              {health && (
                <>
                  <InfoRow label={t('overview.healthStatus')}>
                    <Badge variant={HEALTH_VARIANTS[health.Status] ?? 'secondary'}>{health.Status}</Badge>
                  </InfoRow>
                  <InfoRow label={t('overview.healthFailingStreak')}>{health.FailingStreak}</InfoRow>
                  {health.Log && health.Log.length > 0 && (
                    <InfoRow label={t('overview.healthLastOutput')}>
                      <div className="max-h-24 overflow-y-auto rounded-md bg-muted p-2 font-mono text-xs">
                        {health.Log[health.Log.length - 1]?.Output || '-'}
                      </div>
                    </InfoRow>
                  )}
                </>
              )}
            </>
          ) : (
            <p className="text-sm text-muted-foreground">{t('health.noHealthcheck')}</p>
          )}
        </div>

        {/* Devices */}
        {hc?.Devices && hc.Devices.length > 0 && (
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.devices')}</h3>
            {hc.Devices.map((d) => (
              <div key={`${d.PathOnHost}:${d.PathInContainer}:${d.CgroupPermissions}`} className="flex gap-2 border-b border-border py-1.5 last:border-0">
                <span className="font-mono text-xs text-foreground">{d.PathOnHost}</span>
                <span className="text-xs text-muted-foreground">→</span>
                <span className="font-mono text-xs text-foreground">{d.PathInContainer}</span>
                <span className="font-mono text-xs text-muted-foreground">({d.CgroupPermissions})</span>
              </div>
            ))}
          </div>
        )}

        {/* Ulimits */}
        {hc?.Ulimits && hc.Ulimits.length > 0 && (
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('overview.ulimits')}</h3>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-border">
                    <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitName')}</th>
                    <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitSoft')}</th>
                    <th className="px-2 py-1.5 font-medium text-muted-foreground">{t('overview.ulimitHard')}</th>
                  </tr>
                </thead>
                <tbody>
                  {hc.Ulimits.map((u) => (
                    <tr key={u.Name} className="border-b border-border last:border-0">
                      <td className="px-2 py-1.5 font-mono text-foreground">{u.Name}</td>
                      <td className="px-2 py-1.5 text-foreground">{u.Soft}</td>
                      <td className="px-2 py-1.5 text-foreground">{u.Hard}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
