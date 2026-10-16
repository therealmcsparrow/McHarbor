// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The activity table's seven columns, previously hand-written markup in
// ActivityPage. Cell rendering is unchanged.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate, truncateId } from '@resources/utils/format';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import { type ContainerEvent, parseMeta, ACTION_VARIANTS } from './EventDetails';

export function useActivityColumns(
  envName: (id: string | null) => string,
  onSelect: (event: ContainerEvent) => void,
): ColumnDef<ContainerEvent, unknown>[] {
  const { t } = useTranslation('common');
  const group = t('activity.columnsGroup');

  return useMemo(
    () => [
      {
        id: 'timestamp',
        accessorFn: (evt2) => formatDate(evt2.timestamp),
        header: t('activity.columnTimestamp'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="whitespace-nowrap text-muted-foreground">{formatDate(row.original.timestamp)}</span>
        ),
      },
      {
        id: 'environment',
        accessorFn: (evt2) => evt2.environmentName ?? envName(evt2.environmentId),
        header: t('activity.columnEnvironment'),
        cell: ({ row }) => (
          <span className="text-foreground">{row.original.environmentName ?? envName(row.original.environmentId)}</span>
        ),
      },
      {
        id: 'action',
        accessorFn: (evt2) => evt2.action,
        header: t('activity.columnAction'),
        cell: ({ row }) => (
          <Badge variant={ACTION_VARIANTS[row.original.action] ?? 'secondary'}>{row.original.action}</Badge>
        ),
      },
      {
        id: 'container',
        accessorFn: (entry) => entry.containerName ?? truncateId(entry.containerId),
        header: t('activity.columnContainer'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="max-w-48 truncate font-medium text-foreground">
              {row.original.containerName ?? truncateId(row.original.containerId)}
            </span>
            <span className="font-mono text-xs text-muted-foreground">{truncateId(row.original.containerId)}</span>
          </div>
        ),
      },
      {
        id: 'image',
        accessorFn: (eventItem) => parseMeta(eventItem.metadata).image ?? '-',
        header: t('activity.columnImage'),
        cell: ({ row }) => (
          <span className="block max-w-48 truncate font-mono text-xs text-muted-foreground">
            {parseMeta(row.original.metadata).image ?? '-'}
          </span>
        ),
      },
      {
        id: 'exitCode',
        accessorFn: (evt) => parseMeta(evt.metadata).exitCode ?? '',
        header: t('activity.columnExitCode'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const exitCode = parseMeta(row.original.metadata).exitCode ?? null;
          if (exitCode == null) return <span className="text-muted-foreground">-</span>;
          return <Badge variant={exitCode === '0' ? 'secondary' : 'destructive'}>{exitCode}</Badge>;
        },
      },
      textColumn<ContainerEvent>('eventType', t('activity.columnEventType'), (r) => r.eventType, { group }),
      textColumn<ContainerEvent>('containerId', t('activity.columnContainerId'), (r) => r.containerId?.slice(0, 12), {
        group,
      }),
      dateColumn<ContainerEvent>('createdAt', t('activity.columnCreatedAt'), (r) => r.createdAt, { group }),
      {
        id: 'actions',
        header: t('activity.columnActions'),
        enableSorting: false,
        meta: {
          width: 'xs',
          locked: true,
          noExport: true,
          label: t('activity.columnActions'),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon-sm"
                  onClick={() => onSelect(row.original)}
                  aria-label={t('activity.viewDetails')}
                >
                  <IconEye className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('activity.viewDetails')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [envName, onSelect, t, group],
  );
}
