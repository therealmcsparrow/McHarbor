// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowsExchange, IconDatabase, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { StatusError } from '@resources/components/ui/StatusError';
import { COMPUTE_PERM } from '../../permissions';
import { usePools, useRefreshPool } from '../../hooks/useCompute';
import { CreatePoolDialog } from '../CreatePoolDialog';
import { DeletePoolDialog } from '../DeletePoolDialog';
import { MovePoolDialog } from '../MovePoolDialog';
import { PoolCapacityBar } from '../PoolCapacityBar';
import type { OverviewEntry, Pool } from '../../types';

interface HostPoolsSectionProps {
  entry: OverviewEntry;
}

export function HostPoolsSection({ entry }: HostPoolsSectionProps) {
  const { t } = useTranslation('compute');
  const { data: pools } = usePools(entry.environmentId);
  const refresh = useRefreshPool();
  const [creating, setCreating] = useState(false);
  const [moving, setMoving] = useState<Pool | null>(null);
  const [deleting, setDeleting] = useState<Pool | null>(null);

  const rows = pools ?? [];
  const environments = [{ id: entry.environmentId, name: entry.name }];

  return (
    <Card className="space-y-4 p-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-medium text-foreground">{t('nav.pools')}</h2>
        <Button size="sm" permission={COMPUTE_PERM.MANAGE} onClick={() => setCreating(true)}>
          <IconPlus className="h-4 w-4" aria-hidden="true" />
          {t('actions.createPool')}
        </Button>
      </div>

      {rows.length === 0 ? (
        <EmptyState
          icon={<IconDatabase className="size-6" />}
          title={t('hosts.settings.noPools.title')}
          description={t('hosts.settings.noPools.description')}
        />
      ) : (
        <ul className="space-y-3">
          {rows.map((pool) => (
            <li key={pool.id} className="rounded-lg border border-border p-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="truncate text-sm font-medium text-foreground">{pool.name}</span>
                <Badge variant="default" className="px-1.5 py-0 text-[10px]">
                  {t(`pools.backend.${pool.backend}`)}
                </Badge>
                {pool.isDefault && (
                  <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                    {t('badges.default')}
                  </Badge>
                )}
                <StatusError error={pool.lastError} />
                <div className="ml-auto flex items-center gap-1">
                  <HeaderActionButton
                    tooltip={t('actions.refresh')}
                    icon={<IconRefresh className="size-4" />}
                    permission={COMPUTE_PERM.MANAGE}
                    disabled={refresh.isPending}
                    onClick={() => refresh.mutate(pool.id)}
                  />
                  <HeaderActionButton
                    tooltip={t('hosts.settings.movePool')}
                    icon={<IconArrowsExchange className="size-4" />}
                    permission={COMPUTE_PERM.MANAGE}
                    onClick={() => setMoving(pool)}
                  />
                  <HeaderActionButton
                    tooltip={t('actions.delete')}
                    icon={<IconTrash className="size-4" />}
                    permission={COMPUTE_PERM.DELETE}
                    onClick={() => setDeleting(pool)}
                  />
                </div>
              </div>
              <div className="mt-2">
                <PoolCapacityBar pool={pool} compact />
              </div>
              <p className="mt-1.5 truncate font-mono text-xs text-muted-foreground">{pool.target}</p>
            </li>
          ))}
        </ul>
      )}

      {creating && (
        <CreatePoolDialog
          environments={environments}
          defaultEnvironmentId={entry.environmentId}
          onClose={() => setCreating(false)}
        />
      )}
      {moving && <MovePoolDialog pool={moving} onClose={() => setMoving(null)} />}
      {deleting && <DeletePoolDialog pool={deleting} onClose={() => setDeleting(null)} />}
    </Card>
  );
}
