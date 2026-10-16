// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The audit table's eight columns, as TanStack column defs.
//
// These were hand-written `<th>`/`<td>` markup at the bottom of AuditPage.
// Moving them here is what lets the page use the shared TablePage layout — and
// what gives the columns menu, the density control and CSV/JSON export
// something to work with. Cell rendering is unchanged from the original markup.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import { type AuditEntry, ACTION_VARIANTS, SEVERITY_VARIANTS } from '@resources/components/AuditEntryDetails';

export function useAuditColumns(
  envName: (id: string | null, storedName?: string | null) => string,
  onSelect: (entry: AuditEntry) => void,
): ColumnDef<AuditEntry, unknown>[] {
  const { t } = useTranslation('common');
  const actorGroup = t('audit.columnsGroupActor');
  const entityGroup = t('audit.columnsGroupEntity');
  const generalGroup = t('audit.columnsGroupGeneral');

  return useMemo(
    () => [
      {
        id: 'timestamp',
        accessorFn: (entry) => entry.timestamp,
        header: t('audit.columnTimestamp'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs whitespace-nowrap text-muted-foreground">{row.original.timestamp}</span>
        ),
      },
      {
        id: 'severity',
        accessorFn: (entry) => entry.severity,
        header: t('audit.columnSeverity'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={SEVERITY_VARIANTS[row.original.severity] ?? 'secondary'}>
            {t(`audit.severity.${row.original.severity}`)}
          </Badge>
        ),
      },
      {
        id: 'username',
        accessorFn: (entry) => entry.username ?? '-',
        header: t('audit.columnUser'),
        cell: ({ row }) => (
          <span className="flex flex-col">
            <span className="font-medium text-foreground">{row.original.username ?? '-'}</span>
            {row.original.apiKeyName ? (
              <span className="text-xs text-muted-foreground">
                {t('audit.viaApiKey', { name: row.original.apiKeyName })}
              </span>
            ) : null}
          </span>
        ),
      },
      {
        id: 'action',
        accessorFn: (entry) => entry.action,
        header: t('audit.columnAction'),
        cell: ({ row }) => (
          <Badge variant={ACTION_VARIANTS[row.original.action] ?? 'secondary'}>{row.original.action}</Badge>
        ),
      },
      {
        id: 'entity',
        accessorFn: (entry) => entry.entityName ?? entry.entityType ?? '-',
        header: t('audit.columnEntity'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex flex-col">
            <span className="max-w-48 truncate font-medium text-foreground">
              {row.original.entityName ?? row.original.entityType ?? '-'}
            </span>
            {row.original.entityId && (
              <span className="font-mono text-xs text-muted-foreground">{truncateId(row.original.entityId)}</span>
            )}
          </div>
        ),
      },
      {
        id: 'environment',
        accessorFn: (entry) => envName(entry.environmentId, entry.environmentName),
        header: t('audit.columnEnvironment'),
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {envName(row.original.environmentId, row.original.environmentName)}
          </span>
        ),
      },
      {
        id: 'details',
        accessorFn: (entry) => entry.details ?? '-',
        header: t('audit.columnDetails'),
        cell: ({ row }) => (
          <span className="block max-w-48 truncate text-muted-foreground">{row.original.details ?? '-'}</span>
        ),
      },
      textColumn<AuditEntry>('userId', t('audit.columnUserId'), (r) => r.userId, { group: actorGroup }),
      textColumn<AuditEntry>('entityType', t('audit.columnEntityType'), (r) => r.entityType, { group: entityGroup }),
      textColumn<AuditEntry>('entityId', t('audit.columnEntityId'), (r) => r.entityId, { group: entityGroup }),
      textColumn<AuditEntry>('ipAddress', t('audit.columnIpAddress'), (r) => r.ipAddress, { group: actorGroup }),
      dateColumn<AuditEntry>('createdAt', t('audit.columnCreatedAt'), (r) => r.createdAt, { group: generalGroup }),
      {
        id: 'actions',
        header: t('audit.columnActions'),
        enableSorting: false,
        // Icon buttons have no text form, so this column stays out of exports;
        // `locked` also keeps it out of reach of the columns menu.
        meta: { width: 'xs', locked: true, noExport: true, label: t('audit.columnActions') } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon-sm"
                  onClick={() => onSelect(row.original)}
                  aria-label={t('audit.viewDetails')}
                >
                  <IconEye className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('audit.viewDetails')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [envName, onSelect, t, actorGroup, entityGroup, generalGroup],
  );
}
