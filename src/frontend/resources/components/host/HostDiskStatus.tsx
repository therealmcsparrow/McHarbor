// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabaseOff } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { hostDiskLevel, useHostDiskStatus, type HostDiskLevel } from '@resources/hooks/useHostDiskStatus';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';
import { formatBytes, timeAgo } from '@resources/utils/format';

const BAR_TONE: Record<HostDiskLevel, string> = {
  normal: 'bg-primary',
  warning: 'bg-warning',
  critical: 'bg-danger',
};

const TEXT_TONE: Record<HostDiskLevel, string> = {
  normal: 'text-foreground',
  warning: 'text-warning',
  critical: 'text-danger',
};

type HostDiskStatusProps = {
  envId: string;
  enabled?: boolean;
  className?: string;
};

/** Missing data is grey and worded, never a green bar: an unmeasured disk must not read as a healthy one. */
export function HostDiskStatus({ envId, enabled = true, className }: HostDiskStatusProps) {
  const { t } = useTranslation('host');
  const { data, isLoading, isError, allowed } = useHostDiskStatus(envId, enabled);
  const muted = cn('flex items-center gap-1.5 text-xs text-muted-foreground', className);

  if (!allowed) {
    return <p className={muted}>{t('diskStatus.requires', { permission: PERM.HOST_VIEW })}</p>;
  }
  if (!enabled) return null;
  if (isLoading) return <div className="h-4 w-32 animate-pulse rounded bg-muted" aria-hidden="true" />;
  if (isError || !data) {
    return (
      <p className={muted}>
        <IconDatabaseOff className="size-3.5 shrink-0" aria-hidden="true" />
        {t('diskStatus.error')}
      </p>
    );
  }
  if (data.state === 'unsupported') return <p className={muted}>{t('diskStatus.unsupported')}</p>;
  if (data.state !== 'ok') {
    return (
      <p className={muted}>
        <IconDatabaseOff className="size-3.5 shrink-0" aria-hidden="true" />
        {data.lastSeen ? t('diskStatus.stale', { ago: timeAgo(data.lastSeen) }) : t('diskStatus.unknown')}
      </p>
    );
  }

  const level = hostDiskLevel(data);
  const percent = Math.max(0, Math.min(100, data.percent));
  return (
    <div className={cn('space-y-1', className)}>
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="text-muted-foreground">{t('diskStatus.label')}</span>
        <span className={cn('tabular-nums', TEXT_TONE[level])}>
          {t('diskStatus.usage', { percent: data.percent.toFixed(1), total: formatBytes(data.totalBytes) })}
          {level !== 'normal' && <span className="ml-1">{t(`diskStatus.level.${level}`)}</span>}
        </span>
      </div>
      <meter className="sr-only" aria-label={t('diskStatus.label')} min={0} max={100} value={Math.round(percent)} />
      <div aria-hidden="true" className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div className={cn('h-full w-(--w)', BAR_TONE[level])} style={cssVars({ w: `${percent}%` })} />
      </div>
    </div>
  );
}
