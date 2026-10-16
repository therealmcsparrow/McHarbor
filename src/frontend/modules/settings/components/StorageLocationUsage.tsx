// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { cn } from '@resources/utils/cn';
import { formatBytes } from '@resources/utils/format';
import { useStorageLocationUsage } from '@resources/hooks/useStorageLocations';
import { capacityFillClass } from '@resources/utils/capacity';

type StorageLocationUsageProps = {
  locationId: string;
  enabled: boolean;
};

// Tailwind needs literal class names, so the fill width is chosen
// from a fixed ladder instead of an inline style. 5 % steps are
// fine for a 300px-wide bar.
const WIDTH_STEPS = [
  'w-0',
  'w-[5%]',
  'w-[10%]',
  'w-[15%]',
  'w-[20%]',
  'w-[25%]',
  'w-[30%]',
  'w-[35%]',
  'w-[40%]',
  'w-[45%]',
  'w-[50%]',
  'w-[55%]',
  'w-[60%]',
  'w-[65%]',
  'w-[70%]',
  'w-[75%]',
  'w-[80%]',
  'w-[85%]',
  'w-[90%]',
  'w-[95%]',
  'w-full',
] as const;

function widthClass(percent: number) {
  const step = Math.min(20, Math.max(0, Math.round(percent / 5)));
  // Anything above zero should still be visible.
  if (percent > 0 && step === 0) return WIDTH_STEPS[1];
  return WIDTH_STEPS[step];
}

function fillTone(percent: number) {
  return capacityFillClass(percent);
}

export function StorageLocationUsage({ locationId, enabled }: StorageLocationUsageProps) {
  const { t } = useTranslation('settings');
  const usage = useStorageLocationUsage(locationId, enabled);

  if (!enabled) return null;

  if (usage.isPending) {
    return (
      <div className="mt-2 flex items-center gap-3" aria-busy="true">
        <Skeleton className="h-1.5 w-full max-w-[300px] rounded-full" />
        <span className="text-xs text-muted-foreground">{t('storage.usage.loading')}</span>
      </div>
    );
  }

  if (usage.isError) {
    return <ErrorState variant="inline" className="mt-2" title={t('storage.usage.error')} error={usage.error} />;
  }

  const data = usage.data;
  if (!data || !data.supported) {
    return <p className="mt-2 text-xs text-muted-foreground">{t('storage.usage.unavailable')}</p>;
  }

  const used = formatBytes(data.usedBytes);
  if (data.totalBytes <= 0) {
    return <p className="mt-2 text-xs text-muted-foreground">{t('storage.usage.used', { used })}</p>;
  }

  const total = formatBytes(data.totalBytes);
  const free = formatBytes(data.freeBytes);
  const percent = Math.round(data.percent);
  const summary = t('storage.usage.usedOfTotal', { used, total });

  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
      <div
        title={t('storage.usage.free', { free })}
        className="h-1.5 w-full max-w-[300px] overflow-hidden rounded-full bg-muted"
      >
        <progress
          className="sr-only"
          aria-label={t('storage.usage.label')}
          aria-valuetext={summary}
          value={percent}
          max={100}
        />
        <div
          aria-hidden="true"
          className={cn(
            'h-full rounded-full transition-all duration-300 ease-out',
            fillTone(data.percent),
            widthClass(data.percent),
          )}
        />
      </div>
      <span className="text-xs text-muted-foreground">
        {summary} · {percent}%
      </span>
    </div>
  );
}
