// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconShieldOff } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useWidgetContext } from './mount';

type SnapshotSummary = {
  vmId: string;
  name: string;
  environmentId: string;
  count: number;
  newestAt?: string;
  consistency?: string;
};

function ageDays(iso: string | undefined): number | null {
  if (!iso) return null;
  const parsed = Date.parse(iso);
  if (Number.isNaN(parsed)) return null;
  return Math.floor((Date.now() - parsed) / 86_400_000);
}

/**
 * Which guests have no snapshot, and how stale the newest one is elsewhere.
 *
 * "Unprotected" is the number worth a tile: a guest with no snapshot is one
 * bad upgrade away from being rebuilt by hand, and nothing else on the
 * dashboard says so. The oldest-newest figure catches the other half — a
 * snapshot from four months ago is coverage on paper only.
 */
export function ProtectionWidget() {
  const { t } = useTranslation('vm');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useQuery({
    queryKey: ['vms', 'snapshots', 'summary'],
    queryFn: () => api.get<SnapshotSummary[]>('/vms/snapshots/summary').then((r) => assertSuccess(r) ?? []),
    refetchInterval: 60_000,
  });

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const guests = data ?? [];
  const unprotected = guests.filter((entry) => entry.count === 0);
  const stale = guests
    .filter((entry) => entry.count > 0)
    .map((entry) => ({ entry, days: ageDays(entry.newestAt) }))
    .filter((row): row is { entry: SnapshotSummary; days: number } => row.days !== null)
    .sort((a, b) => b.days - a.days);

  if (guests.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconShieldOff className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('list.empty.title')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-kvm')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-2 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{unprotected.length}</span>
        <span className="text-sm text-muted-foreground">
          {t('widgets.vmProtection.unprotected', { count: guests.length })}
        </span>
      </div>

      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {unprotected.slice(0, 4).map((entry) => (
          <div key={entry.vmId} className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-sm text-foreground">{entry.name}</span>
            <Badge variant="destructive">{t('widgets.vmProtection.none')}</Badge>
          </div>
        ))}
        {stale.slice(0, 4).map(({ entry, days }) => (
          <div key={entry.vmId} className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-sm text-foreground">{entry.name}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t('widgets.vmProtection.age', { count: days })}
            </span>
          </div>
        ))}
      </div>
    </Button>
  );
}
