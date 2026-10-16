// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ContainerInspect } from '@core/types/docker';

const DEFAULT_STOP_SIGNAL = 'SIGTERM';
const DEFAULT_RUNTIME = 'runc';
const DEFAULT_LOG_DRIVER = 'json-file';

export function OverviewRuntimeCard({ container }: { container: ContainerInspect }) {
  const { t } = useTranslation('containers');
  const hc = container.HostConfig;
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('overview.runtime')}
      </h3>
      <InfoRow label={t('overview.stopSignal')}>{container.Config?.StopSignal ?? DEFAULT_STOP_SIGNAL}</InfoRow>
      <InfoRow label={t('overview.runtimeEngine')}>{hc?.Runtime || DEFAULT_RUNTIME}</InfoRow>
      <InfoRow label={t('overview.logDriver')}>{hc?.LogConfig?.Type ?? DEFAULT_LOG_DRIVER}</InfoRow>
      {hc?.LogConfig?.Config && Object.keys(hc.LogConfig.Config).length > 0 && (
        <InfoRow label={t('overview.logOptions')}>
          <div className="space-y-0.5">
            {Object.entries(hc.LogConfig.Config).map(([k, v]) => (
              <div key={k} className="font-mono text-xs">
                <span className="text-muted-foreground">{k}=</span>
                {v}
              </div>
            ))}
          </div>
        </InfoRow>
      )}
      <InfoRow label={t('overview.init')}>{hc?.Init ? t('common:labels.yes') : t('common:labels.no')}</InfoRow>
      {hc?.PidsLimit !== null && hc?.PidsLimit !== undefined && hc.PidsLimit > 0 && (
        <InfoRow label={t('overview.pidsLimit')}>{hc.PidsLimit}</InfoRow>
      )}
    </div>
  );
}
