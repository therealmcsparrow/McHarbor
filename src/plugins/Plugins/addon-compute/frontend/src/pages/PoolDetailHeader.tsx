// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase, IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { StatusError } from '@resources/components/ui/StatusError';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { COMPUTE_PERM } from '../permissions';
import type { Pool } from '../types';

type PoolDetailHeaderProps = {
  pool: Pool;
  hostName: string;
  onRefresh: () => void;
  refreshing: boolean;
};

export function PoolDetailHeader({ pool, hostName, onRefresh, refreshing }: PoolDetailHeaderProps) {
  const { t } = useTranslation('compute');

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconDatabase className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {pool.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={pool.state === 'ready' ? 'success' : 'warning'}
              label={t(`pools.state.${pool.state}`)}
              size="sm"
            />
            <Badge variant="default" className="px-1.5 py-0 text-[10px]">
              {t(`pools.backend.${pool.backend}`)}
            </Badge>
            {pool.isDefault && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {t('badges.default')}
              </Badge>
            )}
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {hostName}
            </span>
            <StatusError error={pool.lastError} />
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex items-center gap-1.5">
          <HeaderActionButton
            tooltip={t('actions.refresh')}
            icon={<IconRefresh className="size-4" />}
            permission={COMPUTE_PERM.MANAGE}
            disabled={refreshing}
            onClick={onRefresh}
          />
        </div>
      </PageHeadActions>
    </>
  );
}
