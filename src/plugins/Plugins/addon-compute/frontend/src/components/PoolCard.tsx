// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase, IconFolderSymlink, IconRefresh, IconServer, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { COMPUTE_PERM } from '../permissions';
import { formatBytes } from '../format';
import { PoolCapacityBar, PoolSpeedBadge } from './PoolCapacityBar';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { StatTile } from './StatTile';
import type { Pool } from '../types';

interface PoolCardProps {
  pool: Pool;
  hostName: string;
  refreshing: boolean;
  onRefresh: (id: string) => void;
  onDelete: (pool: Pool) => void;
  onMove: (pool: Pool) => void;
  onOpen: () => void;
}

/** One storage pool, in the same shape as an environment card. */
export function PoolCard({ pool, hostName, refreshing, onRefresh, onDelete, onMove, onOpen }: PoolCardProps) {
  const { t } = useTranslation('compute');

  return (
    <Card className="min-h-[300px] transition-colors hover:border-primary/40">
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex min-w-0 items-center gap-2">
              <PlainButton
                type="button"
                onClick={onOpen}
                className="truncate rounded text-base font-semibold text-foreground hover:text-primary hover:underline"
              >
                {pool.name}
              </PlainButton>
              {pool.isDefault && (
                <Badge variant="secondary" className="shrink-0 px-2 py-1 text-[10px]">
                  {t('badges.default')}
                </Badge>
              )}
            </div>
            <p className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
              <IconServer className="size-3.5 shrink-0" aria-hidden="true" />
              {hostName}
            </p>
          </div>
          <PoolSpeedBadge pool={pool} />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">{t(`pools.backend.${pool.backend}`)}</Badge>
          <code className="truncate font-mono text-xs text-muted-foreground">{pool.target}</code>
        </div>

        <div className="rounded-lg border border-border/70 bg-background/40 p-3">
          <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <IconDatabase className="size-4" aria-hidden="true" />
            {t('pools.columns.capacity')}
          </div>
          <div className="mt-3">
            <PoolCapacityBar pool={pool} />
          </div>
        </div>

        {pool.lastError && <p className="text-xs break-words text-destructive">{pool.lastError}</p>}

        <div className="mt-auto grid grid-cols-2 gap-2">
          <StatTile label={t('pools.columns.volumes')} value={pool.volumeCount} />
          <StatTile label={t('pools.columns.capacity')} value={formatBytes(pool.capacityBytes)} />
        </div>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.refresh')}
          title={t('actions.refresh')}
          permission={COMPUTE_PERM.VIEW}
          disabled={refreshing}
          onClick={() => onRefresh(pool.id)}
        >
          <IconRefresh className="h-4 w-4" aria-hidden="true" />
        </Button>
        <div className="flex-1" />
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('pools.move.action')}
          title={t('pools.move.action')}
          permission={COMPUTE_PERM.MANAGE}
          disabled={pool.backend !== 'dir'}
          onClick={() => onMove(pool)}
        >
          <IconFolderSymlink className="h-4 w-4" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.delete')}
          title={t('actions.delete')}
          permission={COMPUTE_PERM.DELETE}
          onClick={() => onDelete(pool)}
        >
          <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
        </Button>
      </CardFooter>
    </Card>
  );
}
