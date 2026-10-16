// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconBolt } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatBytes, isOvercommitted, usedPercent } from '../format';
import type { Pool } from '../types';

/**
 * A pool's capacity, and the two things about it that predict an outage.
 *
 * The bar itself is the easy part. What matters is that both warnings are
 * visible before they become incidents:
 *
 *   - A pool near full. When one actually fills, QEMU pauses every guest on it
 *     with an IO error, which presents as "all my VMs froze" and points nowhere
 *     near the pool.
 *   - Overcommit on a thin backend. Provisioning 4 TiB of guest disks onto 1 TiB
 *     of real space is a legitimate thing to do and the usual reason a pool
 *     fills unexpectedly, so it is stated rather than left to be inferred.
 */

/** Above this, the bar turns. Chosen to leave room to act, not to alarm. */
const WARN_PERCENT = 80;
const DANGER_PERCENT = 90;

interface PoolCapacityBarProps {
  pool: Pool;
  /** Compact hides the byte labels, for a table cell. */
  compact?: boolean;
}

export function PoolCapacityBar({ pool, compact = false }: PoolCapacityBarProps) {
  const { t } = useTranslation('compute');

  const percent = usedPercent(pool.usedBytes, pool.capacityBytes);
  const overcommitted = isOvercommitted(pool.allocatedBytes, pool.capacityBytes);

  const tone = percent >= DANGER_PERCENT ? 'bg-destructive' : percent >= WARN_PERCENT ? 'bg-warning' : 'bg-primary';

  return (
    <div className="min-w-0">
      <progress className="sr-only" value={percent} max={100} aria-label={t('pools.columns.capacity')} />
      <div className="h-2 w-full overflow-hidden rounded-full bg-muted" aria-hidden="true">
        {/*
          Tailwind cannot generate a class for a runtime percentage, and the
          project forbids inline styles — so the width comes from a fixed step
          scale. Twenty steps is finer than the eye resolves on a table-width
          bar and keeps every class statically present in the bundle.
        */}
        <div className={`h-full ${tone} ${widthClass(percent)}`} />
      </div>

      {!compact && (
        <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span>
            {formatBytes(pool.usedBytes)} / {formatBytes(pool.capacityBytes)}
          </span>

          {percent >= WARN_PERCENT && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex items-center gap-1 text-warning">
                  <IconAlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
                  {percent}%
                </span>
              </TooltipTrigger>
              <TooltipContent>{t('pools.capacityWarning', { percent })}</TooltipContent>
            </Tooltip>
          )}

          {overcommitted && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="warning">{formatBytes(pool.allocatedBytes)}</Badge>
              </TooltipTrigger>
              <TooltipContent>
                {t('pools.overcommit', {
                  allocated: formatBytes(pool.allocatedBytes),
                  capacity: formatBytes(pool.capacityBytes),
                })}
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Instant-clone marker.
 *
 * This is the single most consequential thing about a pool's backend, and it is
 * invisible until an operator waits three minutes for a clone. Labelling it on
 * the pool itself is what lets them choose knowingly.
 */
export function PoolSpeedBadge({ pool }: { pool: Pool }) {
  const { t } = useTranslation('compute');

  if (pool.instant) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="success">
            <IconBolt className="h-3 w-3" aria-hidden="true" />
            {t('pools.instant.label')}
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{t('pools.instant.tooltip')}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="secondary">{t('pools.slowClones.label')}</Badge>
      </TooltipTrigger>
      <TooltipContent>{t('pools.slowClones.tooltip')}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Maps a percentage onto one of twenty static width classes.
 *
 * Arbitrary values like `w-[73%]` are generated at build time from source text,
 * so a runtime-computed one produces no class at all and the bar renders empty.
 */
function widthClass(percent: number): string {
  const step = Math.round(percent / 5) * 5;
  switch (step) {
    case 0:
      return 'w-0';
    case 5:
      return 'w-[5%]';
    case 10:
      return 'w-[10%]';
    case 15:
      return 'w-[15%]';
    case 20:
      return 'w-1/5';
    case 25:
      return 'w-1/4';
    case 30:
      return 'w-[30%]';
    case 35:
      return 'w-[35%]';
    case 40:
      return 'w-2/5';
    case 45:
      return 'w-[45%]';
    case 50:
      return 'w-1/2';
    case 55:
      return 'w-[55%]';
    case 60:
      return 'w-3/5';
    case 65:
      return 'w-[65%]';
    case 70:
      return 'w-[70%]';
    case 75:
      return 'w-3/4';
    case 80:
      return 'w-4/5';
    case 85:
      return 'w-[85%]';
    case 90:
      return 'w-[90%]';
    case 95:
      return 'w-[95%]';
    default:
      return 'w-full';
  }
}
