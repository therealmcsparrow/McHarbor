// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The backup-log table's eight columns. Previously the `<thead>` in
// BackupLogsTab plus the `<tr>` in BackupLogRow; cell rendering is unchanged.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { formatDate } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { cn } from '@resources/utils/cn';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn } from '@resources/layout/table-page/optional-columns';
import type { BackupLog } from '../hooks/useBackupLogs';
import { RunSourceCell } from '../pages/backup-source-cell';
import { useSourceLabel } from '../hooks/useSourceLabel';

const DASH = '—';

export function useBackupLogColumns(): ColumnDef<BackupLog, unknown>[] {
  const { t } = useTranslation('containers');
  const formatPrefs = useLocaleFormat();
  const sourceLabel = useSourceLabel();
  const group = t('backups.logs.columnsGroup');

  return useMemo(
    () => [
      {
        id: 'time',
        accessorFn: (log) => formatDate(log.createdAt, formatPrefs),
        header: t('backups.logs.table.time'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs whitespace-nowrap text-muted-foreground">
            {formatDate(row.original.createdAt, formatPrefs)}
          </span>
        ),
      },
      {
        id: 'severity',
        accessorFn: (log) => log.severity,
        header: t('backups.logs.table.severity'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const style = severityStyle(row.original.severity);
          const label = t(`backups.logs.severity.${row.original.severity}`);
          return (
            <span
              className={cn(
                'inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-medium capitalize',
                style.pill,
                style.text,
              )}
              aria-label={label}
            >
              <span className={cn('size-1.5 rounded-full', style.dot)} aria-hidden="true" />
              {label}
            </span>
          );
        },
      },
      {
        id: 'action',
        accessorFn: (log) => log.action,
        header: t('backups.logs.table.action'),
        cell: ({ row }) =>
          // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: backup log action codes are written by the backend and can be extended
          t(`backups.logs.action.${row.original.action}`, { defaultValue: row.original.action }),
      },
      {
        id: 'message',
        accessorFn: (log) => log.message || DASH,
        header: t('backups.logs.table.message'),
      },
      {
        id: 'source',
        accessorFn: (log) => sourceLabel({ sourceKind: log.subjectKind }),
        header: t('backups.runs.table.source'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <RunSourceCell
            row={{ sourceKind: row.original.subjectKind }}
            label={sourceLabel({ sourceKind: row.original.subjectKind })}
          />
        ),
      },
      {
        id: 'container',
        accessorFn: (log) => log.containerName || log.containerId?.slice(0, 12) || DASH,
        header: t('backups.runs.table.subject'),
      },
      {
        id: 'plan',
        accessorFn: (log) => log.planName || log.planId?.slice(0, 12) || log.refPlanId?.slice(0, 12) || DASH,
        header: t('backups.logs.table.plan'),
      },
      {
        id: 'environment',
        accessorFn: (log) => (log.environmentName ?? log.environmentId?.slice(0, 8)) || DASH,
        header: t('backups.logs.table.environment'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">
            {(row.original.environmentName ?? row.original.environmentId?.slice(0, 8)) || DASH}
          </span>
        ),
      },
      {
        id: 'phase',
        accessorFn: (log) => log.phase || DASH,
        header: t('backups.logs.table.phase'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.phase || DASH}</span>
        ),
      },
      textColumn<BackupLog>('runId', t('backups.logs.table.runId'), (r) => r.runId ?? r.refRunId, { group }),
      textColumn<BackupLog>('logSource', t('backups.logs.table.logSource'), (r) => r.source, { group, width: 'sm' }),
      textColumn<BackupLog>('planId', t('backups.logs.table.planId'), (r) => r.planId ?? r.refPlanId, { group }),
    ],
    [formatPrefs, t, sourceLabel, group],
  );
}
