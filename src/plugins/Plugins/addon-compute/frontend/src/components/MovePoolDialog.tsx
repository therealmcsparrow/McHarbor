// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconArrowRight } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { COMPUTE_PERM } from '../permissions';
import { formatBytes } from '../format';
import { PoolMoveProgress } from './PoolMoveProgress';
import { usePoolMovePlan, usePoolMoveStatus, useRepairPool, useStartPoolMove } from '../hooks/usePoolMove';
import type { Pool } from '../types';

export function MovePoolDialog({ pool, onClose }: { pool: Pool; onClose: () => void }) {
  const { t } = useTranslation('compute');
  const [target, setTarget] = useState('');
  const [asked, setAsked] = useState('');
  const plan = usePoolMovePlan(pool.id, asked);
  const start = useStartPoolMove();
  const repair = useRepairPool();
  const [started, setStarted] = useState(false);
  const moveStatus = usePoolMoveStatus(pool.id, started);

  const running = moveStatus.data?.state === 'running';
  const finished = moveStatus.data?.state === 'done';
  const failed = moveStatus.data?.state === 'failed';

  if (started && (finished || failed)) setStarted(false);

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && !running && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('pools.move.title', { name: pool.name })}</DialogTitle>
          <DialogDescription>{t('pools.move.body')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-1.5">
          <Label htmlFor="pool-move-target">{t('pools.move.target')}</Label>
          <div className="flex gap-2">
            <Input
              id="pool-move-target"
              value={target}
              placeholder={pool.target}
              autoComplete="off"
              spellCheck={false}
              onChange={(ev) => setTarget(ev.target.value)}
            />
            <Button variant="outline" disabled={!target.trim() || running} onClick={() => setAsked(target.trim())}>
              {t('pools.move.check')}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">{t('pools.move.current', { path: pool.target })}</p>
        </div>

        {plan.isFetching ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" />
            {t('pools.move.planning')}
          </div>
        ) : null}

        {plan.data && plan.data.from !== plan.data.to ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2 rounded-md border border-border p-3 text-xs">
              <span className="truncate font-mono text-muted-foreground">{plan.data.from}</span>
              <IconArrowRight className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span className="truncate font-mono text-foreground">{plan.data.to}</span>
            </div>
            <p className="text-sm text-muted-foreground">
              {t('pools.move.footprint', {
                entries: plan.data.entries,
                size: formatBytes(plan.data.bytes),
              })}
            </p>
            {plan.data.runningGuests.length > 0 ? (
              <Alert
                variant="warning"
                icon={<IconAlertTriangle className="size-4 shrink-0" aria-hidden="true" />}
                title={t('pools.move.willStop.title', {
                  count: plan.data.runningGuests.length,
                })}
                description={t('pools.move.willStop.body')}
              />
            ) : null}
          </div>
        ) : null}

        {moveStatus.data && (running || finished || failed) ? <PoolMoveProgress data={moveStatus.data} /> : null}

        <div className="rounded-md border border-border p-3">
          <p className="text-xs text-muted-foreground">{t('pools.move.repairHint')}</p>
          <div className="mt-2 flex flex-wrap items-center gap-3">
            <Button
              size="sm"
              variant="outline"
              permission={COMPUTE_PERM.MANAGE}
              disabled={running || repair.isPending}
              onClick={() => repair.mutate(pool.id)}
            >
              {repair.isPending && <Spinner className="mr-2 size-4" />}
              {t('pools.move.repair')}
            </Button>
            {repair.data ? (
              <span className="text-xs text-success">
                {t('pools.move.repaired', {
                  count: repair.data.moved?.length ?? 0,
                })}
              </span>
            ) : null}
            {repair.data?.notes?.length ? (
              <span className="text-xs text-muted-foreground">{repair.data.notes.join(' · ')}</span>
            ) : null}
            {repair.data?.failures?.length ? (
              <StatusError
                error={repair.data.failures.map((failure) => `${failure.path}: ${failure.reason}`).join(' | ')}
              />
            ) : null}
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" disabled={running} onClick={onClose}>
            {t('actions.close', { ns: 'common' })}
          </Button>
          <Button
            variant="destructive"
            permission={COMPUTE_PERM.MANAGE}
            disabled={running || start.isPending || !plan.data || plan.data.from === plan.data.to}
            onClick={() => {
              if (!plan.data) return;
              setStarted(true);
              start.mutate({ poolId: pool.id, target: plan.data.to });
            }}
          >
            {(running || start.isPending) && <Spinner className="mr-2 size-4" />}
            {t('pools.move.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
