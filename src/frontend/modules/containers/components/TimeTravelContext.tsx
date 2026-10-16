// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ContainerInspect } from '@core/types/docker';
import { useAuditLogs } from '@resources/hooks/useAuditLogs';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Spinner } from '@resources/components/ui/Spinner';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { formatDate } from '@resources/utils/format';

type TimeTravelContextProps = {
  container: ContainerInspect;
  timestamp: string;
};

export function TimeTravelContext({ container, timestamp }: TimeTravelContextProps) {
  const { t } = useTranslation('containers');
  const at = new Date(timestamp);
  const enabled = !Number.isNaN(at.getTime());
  const before = useAuditLogs('', 'container', { from: null, to: enabled ? at : null }, container.Id, enabled);
  const after = useAuditLogs(
    '',
    'container',
    { from: enabled ? new Date(at.getTime() + 1) : null, to: null },
    container.Id,
    enabled,
  );
  if (!enabled) return null;

  const latestBefore = before.data?.pages[0]?.items[0];
  const laterChanges = after.data?.pages[0]?.total ?? 0;
  const envKeys = (container.Config?.Env ?? []).map((entry) => entry.split('=', 1)[0] ?? '').filter(Boolean);

  return (
    <section className="mt-5 border-t border-border pt-5">
      <h3 className="mb-3 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('timeTravel.title')}
      </h3>
      {before.isLoading || after.isLoading ? (
        <output className="flex h-20 items-center justify-center">
          <Spinner size="sm" />
        </output>
      ) : before.error || after.error ? (
        <ErrorState
          variant="inline"
          error={before.error ?? after.error}
          onRetry={() => {
            void Promise.all([before.refetch(), after.refetch()]);
          }}
        />
      ) : laterChanges > 0 ? (
        <Alert variant="warning" description={t('timeTravel.changedWarning', { count: laterChanges })} />
      ) : (
        <Alert variant="info" description={t('timeTravel.noLaterChanges')} />
      )}
      <p className="my-3 text-xs text-muted-foreground">{t('timeTravel.referenceDescription')}</p>
      <div className="space-y-1">
        <InfoRow label={t('timeTravel.image')}>
          <span className="font-mono text-xs break-all">{container.Config?.Image || container.Image}</span>
        </InfoRow>
        <InfoRow label={t('timeTravel.mounts')}>
          <span className="space-y-1 font-mono text-xs">
            {(container.Mounts ?? []).map((mount) => (
              <span key={`${mount.Type}:${mount.Destination}`} className="block break-all">
                {mount.Type} → {mount.Destination}
              </span>
            ))}
          </span>
        </InfoRow>
        <InfoRow label={t('timeTravel.environmentKeys')}>
          <span className="flex flex-wrap gap-1">
            {envKeys.map((key) => (
              <Badge key={key} variant="secondary" className="font-mono text-xs">
                {key}
              </Badge>
            ))}
          </span>
        </InfoRow>
        {latestBefore ? (
          <InfoRow label={t('timeTravel.lastChange')}>
            <span className="text-xs">
              {latestBefore.action} · {formatDate(latestBefore.timestamp)}
            </span>
          </InfoRow>
        ) : null}
      </div>
    </section>
  );
}
