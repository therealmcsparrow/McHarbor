// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { formatDate } from '@resources/utils/format';
import { textColumn } from '@resources/layout/table-page/optional-columns';
import type { APIKeyUsageEvent } from '../hooks/useAPIKeyUsage';
import { formatCount } from '../utils/formatCount';

/** Status classes get the tone an operator expects at a glance. */
function statusVariant(statusValue: number): 'success' | 'warning' | 'destructive' | 'secondary' {
  if (statusValue >= 500) return 'destructive';
  if (statusValue >= 400) return 'warning';
  if (statusValue >= 200 && statusValue < 400) return 'success';
  return 'secondary';
}

/** A long value that must stay on one line: truncate, reveal on hover. */
function Truncated({ value, mono }: { value: string; mono?: boolean }) {
  if (!value) return <span className="text-xs text-muted-foreground">—</span>;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className={mono ? 'block truncate font-mono text-xs' : 'block truncate text-xs'}>{value}</span>
        </TooltipTrigger>
        <TooltipContent className="max-w-md break-all">{value}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

/**
 * Columns for the per-key request history.
 *
 * Client and peer addresses are separate columns on purpose. Behind a
 * reverse proxy they differ, and collapsing them would let an operator read
 * the proxy as the origin of every request. The source column says which
 * signal produced the client value, so an unattributed request is visibly
 * unattributed rather than silently wrong.
 */
export function useAPIKeyUsageColumns(environmentName: (id: string | null) => string) {
  const { t } = useTranslation('security');
  const group = t('columns.groupRequest');

  return useMemo<ColumnDef<APIKeyUsageEvent, unknown>[]>(
    () => [
      {
        id: 'occurredAt',
        accessorFn: (evt2) => formatDate(evt2.occurredAt),
        header: t('apiKeys.usage.when'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      {
        id: 'method',
        accessorFn: (evt2) => evt2.method,
        header: t('apiKeys.usage.method'),
        meta: { width: 'xs' } satisfies TableColumnMeta,
        cell: ({ row }) => <code className="text-xs font-medium">{row.original.method}</code>,
      },
      {
        id: 'routePattern',
        accessorFn: (evt2) => evt2.routePattern,
        header: t('apiKeys.usage.endpoint'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => <Truncated value={row.original.routePattern} mono />,
      },
      {
        id: 'statusCode',
        accessorFn: (evt2) => evt2.statusCode,
        header: t('apiKeys.usage.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex items-center gap-1.5">
            <Badge variant={statusVariant(row.original.statusCode)}>{row.original.statusCode}</Badge>
            {row.original.authOutcome !== 'accepted' && (
              <Badge variant="destructive">{t(`apiKeys.usage.outcome.${row.original.authOutcome}`)}</Badge>
            )}
          </div>
        ),
      },
      {
        id: 'durationMs',
        accessorFn: (evt2) => evt2.durationMs,
        header: t('apiKeys.usage.duration'),
        meta: { width: 'xs' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="text-xs">
            {formatCount(row.original.durationMs)} {t('apiKeys.usage.ms')}
          </span>
        ),
      },
      {
        id: 'environmentId',
        accessorFn: (evt2) => environmentName(evt2.environmentId),
        header: t('apiKeys.usage.environment'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      {
        id: 'clientIp',
        accessorFn: (evt2) => evt2.clientIp,
        header: t('apiKeys.usage.clientIp'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <Truncated value={row.original.clientIp} mono />,
      },
      {
        id: 'peerIp',
        accessorFn: (entry) => entry.peerIp,
        header: t('apiKeys.usage.peerIp'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <Truncated value={row.original.peerIp} mono />,
      },
      {
        id: 'clientIpSource',
        accessorFn: (eventItem) => eventItem.clientIpSource,
        header: t('apiKeys.usage.ipSource'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const proxied = row.original.clientIpSource !== 'peer';
          return (
            <Badge variant={proxied ? 'warning' : 'secondary'}>
              {proxied ? t('apiKeys.usage.proxied') : t('apiKeys.usage.direct')}
            </Badge>
          );
        },
      },
      {
        id: 'userAgent',
        accessorFn: (evt) => evt.userAgent,
        header: t('apiKeys.usage.userAgent'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => <Truncated value={row.original.userAgent} mono />,
      },
      textColumn<APIKeyUsageEvent>('eventId', t('columns.id'), (r) => r.id, { group }),
      textColumn<APIKeyUsageEvent>('authOutcome', t('columns.authOutcome'), (r) => r.authOutcome, {
        group,
        width: 'sm',
      }),
    ],
    [t, environmentName, group],
  );
}
