// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useOverview } from '../hooks/useCompute';
import { formatBytes, usedPercent } from '../format';
import { PoolCapacityBar } from '../components/PoolCapacityBar';
import { useWidgetContext } from './mount';
import type { Pool } from '../types';

/**
 * Storage pool usage across every compute host, fullest first.
 *
 * Ordering by fullness rather than by name is the whole point of the tile: a
 * dashboard is glanced at, and the pool about to cause an incident is the one
 * that should be under the reader's eye. A pool that fills pauses every guest
 * on it with an IO error, which presents as "all my VMs froze" and points
 * nowhere near the pool.
 */
export function CapacityWidget() {
  const { t } = useTranslation('compute');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useOverview();

  if (isLoading) {
    return <Skeleton className="h-full w-full" />;
  }

  const pools: { host: string; pool: Pool }[] = [];
  for (const entry of data?.environments ?? []) {
    if (!entry.computeEnabled || !entry.available) continue;
    for (const pool of entry.pools ?? []) {
      pools.push({ host: entry.name || entry.environmentId, pool });
    }
  }

  pools.sort(
    (a, b) => usedPercent(b.pool.usedBytes, b.pool.capacityBytes) - usedPercent(a.pool.usedBytes, a.pool.capacityBytes),
  );

  if (pools.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconDatabase className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('pools.empty.title')}</p>
      </div>
    );
  }

  const totalCapacity = pools.reduce((sum, entry) => sum + entry.pool.capacityBytes, 0);
  const totalUsed = pools.reduce((sum, entry) => sum + entry.pool.usedBytes, 0);

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-2xl font-semibold text-foreground">{formatBytes(totalUsed)}</span>
        <span className="text-sm text-muted-foreground">/ {formatBytes(totalCapacity)}</span>
      </div>

      {/* Only the top few: a dashboard tile that scrolls is one nobody reads. */}
      <ul className="min-h-0 flex-1 space-y-2 overflow-y-auto">
        {pools.slice(0, 5).map(({ host, pool }) => (
          <li key={pool.id}>
            <Button
              type="button"
              variant="ghost"
              size="lg"
              onClick={() => navigate('/plugins/addon-compute/pools')}
              className="flex w-full flex-col items-stretch justify-start rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
            >
              <div className="flex items-baseline justify-between gap-2 text-xs">
                <span className="truncate text-foreground">{pool.name}</span>
                <span className="shrink-0 text-muted-foreground">{host}</span>
              </div>
              <div className="mt-1">
                <PoolCapacityBar pool={pool} compact />
              </div>
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
