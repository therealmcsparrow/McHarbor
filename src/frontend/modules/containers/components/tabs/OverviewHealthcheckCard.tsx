// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { containerHealthPresentation } from '@resources/utils/container-state';
import type { ContainerInspect } from '@core/types/docker';

export function OverviewHealthcheckCard({ container }: { container: ContainerInspect }) {
  const { t } = useTranslation('containers');
  const health = container.State?.Health;
  const healthcheck = container.Config?.Healthcheck;
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('overview.healthcheck')}
      </h3>
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
                <Badge variant={containerHealthPresentation(health.Status).badgeVariant}>{health.Status}</Badge>
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
  );
}
