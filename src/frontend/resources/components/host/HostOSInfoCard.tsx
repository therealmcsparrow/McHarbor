// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Hint } from '@resources/components/ui/Hint';
import type { HostOSInfo } from '@resources/hooks/useHostUpdates';

type HostOSInfoCardProps = {
  info?: HostOSInfo;
  loading: boolean;
  error: boolean;
};

type UptimeKey =
  | 'detail.hostUpdates.uptimeDays'
  | 'detail.hostUpdates.uptimeHours'
  | 'detail.hostUpdates.uptimeMinutes';

function uptimeParts(seconds: number | undefined): [UptimeKey, Record<string, number>] | null {
  if (!seconds || seconds <= 0) return null;
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) return ['detail.hostUpdates.uptimeDays', { days, hours }];
  if (hours > 0) return ['detail.hostUpdates.uptimeHours', { hours, minutes }];
  return ['detail.hostUpdates.uptimeMinutes', { minutes }];
}

export function HostOSInfoCard({ info, loading, error }: HostOSInfoCardProps) {
  const { t } = useTranslation('environments');
  const uptime = uptimeParts(info?.uptimeSeconds);

  const fields: Array<{ label: string; value: string }> = [
    {
      label: t('detail.hostUpdates.os'),
      value: info?.prettyName || info?.distro || '-',
    },
    { label: t('detail.hostUpdates.osVersion'), value: info?.version || '-' },
    { label: t('detail.hostUpdates.kernel'), value: info?.kernel || '-' },
    { label: t('detail.hostUpdates.architecture'), value: info?.arch || '-' },
    { label: t('detail.hostUpdates.hostname'), value: info?.hostname || '-' },
    {
      label: t('detail.hostUpdates.packageManager'),
      value: info?.manager || '-',
    },
    {
      label: t('detail.hostUpdates.virtualization'),
      value: info?.virtualization || '-',
    },
    {
      label: t('detail.hostUpdates.uptime'),
      value: uptime ? t(uptime[0], uptime[1]) : '-',
    },
  ];

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-foreground">{t('detail.hostUpdates.osTitle')}</h3>
        {info?.rebootRequired ? <Badge variant="warning">{t('detail.hostUpdates.rebootRequired')}</Badge> : null}
      </div>

      {info?.unsupported ? (
        <Alert variant="warning" description={info.unsupported} />
      ) : error ? (
        <Alert variant="destructive" description={t('detail.hostUpdates.osFailed')} />
      ) : loading ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {fields.map((field) => (
            <Skeleton key={field.label} className="h-10 w-full" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
          {fields.map((field) => (
            <div key={field.label}>
              <span className="text-muted-foreground">{field.label}</span>
              <Hint text={field.value}>
                <p className="truncate font-medium text-foreground">{field.value}</p>
              </Hint>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
