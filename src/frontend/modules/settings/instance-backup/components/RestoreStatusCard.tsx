// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Card } from '@resources/components/ui/Card';
import { StatusError } from '@resources/components/ui/StatusError';
import { formatDate } from '@resources/utils/format';
import type { RestoreState, RestoreStatus } from '../hooks/useInstanceRestore';

const VARIANT: Record<RestoreState, 'success' | 'destructive' | 'info' | 'warning'> = {
  running: 'info',
  restarting: 'warning',
  reconciling: 'warning',
  done: 'success',
  failed: 'destructive',
};

export function RestoreStatusCard({ status: currentStatus }: { status: RestoreStatus }) {
  const { t } = useTranslation('instancebackup');
  const failed = (currentStatus.plugins ?? []).filter((p) => p.action === 'failed');
  const refusal = currentStatus.refusals?.length
    ? currentStatus.refusals.map((r) => t(`plan.refusals.${r}`)).join(' ')
    : currentStatus.error;

  return (
    <Card className="flex flex-col gap-2 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium text-foreground">{t('status.title')}</span>
        <Badge variant={VARIANT[currentStatus.state]}>{t(`restoreState.${currentStatus.state}`)}</Badge>
        <StatusError error={refusal ?? ''} title={t('status.errorTitle')} />
        <span className="text-xs text-muted-foreground">
          {t('status.by', { actor: currentStatus.actor, date: formatDate(currentStatus.startedAt) })}
        </span>
      </div>
      {currentStatus.state === 'running' && currentStatus.progress && currentStatus.progress.total > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('progress', { done: currentStatus.progress.done, total: currentStatus.progress.total })}
        </p>
      )}
      {(currentStatus.state === 'restarting' || currentStatus.state === 'reconciling') && (
        <p className="text-xs text-muted-foreground">{t(`status.${currentStatus.state}`)}</p>
      )}
      {currentStatus.result && currentStatus.state === 'done' && (
        <p className="text-xs text-muted-foreground">
          {t('status.result', { rows: currentStatus.result.rows, tables: currentStatus.result.tables })}
          {currentStatus.result.anchorId ? ` ${t('status.anchor')}` : ''}
        </p>
      )}
      {(currentStatus.plugins ?? []).length > 0 && (
        <ul className="flex flex-col gap-1 text-xs">
          {(currentStatus.plugins ?? []).map((p) => (
            <li key={p.slug} className="flex items-center gap-2">
              <span className="font-mono">
                {p.slug}@{p.version}
              </span>
              <Badge variant={p.action === 'failed' ? 'destructive' : 'secondary'}>
                {t(`pluginAction.${p.action}`)}
              </Badge>
              <StatusError error={p.error ?? ''} title={t('status.pluginErrorTitle', { slug: p.slug })} />
            </li>
          ))}
        </ul>
      )}
      {failed.length > 0 && (
        <p className="text-xs text-destructive">{t('status.pluginsFailed', { count: failed.length })}</p>
      )}
    </Card>
  );
}
