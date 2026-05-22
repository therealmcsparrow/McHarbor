// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { formatBytes } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';

type SettingsTabProps = {
  container: ContainerInspect;
};

export function SettingsTab({ container }: SettingsTabProps) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;
  const exposedPorts = container.Config?.ExposedPorts
    ? Object.keys(container.Config.ExposedPorts)
    : [];

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      {/* Restart Policy */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('settings.restartPolicy')}</h3>
        <InfoRow label={t('settings.policy')}>{hc?.RestartPolicy?.Name ?? 'no'}</InfoRow>
        <InfoRow label={t('settings.maxRetries')}>{hc?.RestartPolicy?.MaximumRetryCount ?? 0}</InfoRow>
      </div>

      {/* Resource Limits */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('settings.resourceLimits')}</h3>
        <InfoRow label={t('settings.memoryLimit')}>
          {hc?.Memory ? formatBytes(hc.Memory) : t('settings.unlimited')}
        </InfoRow>
        <InfoRow label={t('settings.cpuNanoCpus')}>
          {hc?.NanoCpus ? t('settings.cores', { count: Number((hc.NanoCpus / 1e9).toFixed(2)) }) : t('settings.unlimited')}
        </InfoRow>
        <InfoRow label={t('settings.blkioWeight')}>{hc?.BlkioWeight || 0}</InfoRow>
      </div>

      {/* Runtime Config */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('settings.runtimeConfig')}</h3>
        <InfoRow label={t('settings.hostname')}>{container.Config?.Hostname ?? '-'}</InfoRow>
        <InfoRow label={t('settings.workingDir')}>{container.Config?.WorkingDir || '/'}</InfoRow>
        <InfoRow label={t('settings.networkMode')}>{hc?.NetworkMode ?? '-'}</InfoRow>
      </div>

      {/* Exposed Ports */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('settings.exposedPorts')}</h3>
        {exposedPorts.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {exposedPorts.map((port) => (
              <span
                key={port}
                className="rounded-md bg-muted px-2 py-1 font-mono text-xs text-foreground"
              >
                {port}
              </span>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('settings.noExposedPorts')}</p>
        )}
      </div>
    </div>
  );
}
