// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatBytes } from '../format';
import type { usePoolMoveStatus } from '../hooks/usePoolMove';

type MoveStatusData = NonNullable<ReturnType<typeof usePoolMoveStatus>['data']>;

export function PoolMoveProgress({ data }: { data: MoveStatusData }) {
  const { t } = useTranslation('compute');
  const finished = data.state === 'done';
  const failed = data.state === 'failed';

  return (
    <div className="space-y-1.5">
      <ProgressBar value={data.percent} />
      <p className="text-xs text-muted-foreground">
        {t('pools.move.progress', {
          percent: data.percent,
          moved: formatBytes(data.bytesMoved),
          total: formatBytes(data.bytesTotal),
        })}
      </p>
      {finished ? <p className="text-xs text-success">{t('pools.move.finished', { count: data.moved })}</p> : null}
      {failed ? (
        <div className="flex items-center gap-2">
          <p className="text-xs text-destructive">{t('pools.move.failed')}</p>
          <StatusError error={data.error} />
        </div>
      ) : null}
      {data.stoppedGuests?.length ? (
        <p className="text-xs text-muted-foreground">
          {t('pools.move.stopped', {
            count: data.stoppedGuests.length,
          })}
        </p>
      ) : null}
    </div>
  );
}
