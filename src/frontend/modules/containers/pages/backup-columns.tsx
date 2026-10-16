// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Column definitions for the backup *runs* table. Previously hand-written
// `<thead>`/`<tbody>` markup inside backups-page-tables.tsx; cell rendering is
// unchanged, including the per-button `data-testid` hooks. The cell bodies
// live in backup-run-cells.tsx and the plans table in backup-plan-columns.tsx.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { useStorageAdapters } from '@resources/hooks/useStorageAdapters';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { isContainerRun, type BackupRunRow } from './backup-run-rows';
import { PluginRunActionsCell } from './plugin-backup-actions';
import { textColumn, dateColumn, boolColumn } from '@resources/layout/table-page/optional-columns';
import { RunSourceCell } from './backup-source-cell';
import { useSourceLabel } from '../hooks/useSourceLabel';
import { DASH, RunContainerCell, RunStatusCell, RunTypeCell } from './backup-run-cells';
import { RunActionsCell, RunDestinationsCell } from './backup-run-row-cells';
import { environmentName, formatBytes, formatTimestamp, runTypeKey } from './backups-page-helpers';

type RunColumnArgs = {
  envNameById: Map<string, string>;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onDownload: (run: ContainerBackupRun) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
  // Open the "what did this run contain" modal, and the "why did it fail"
  // modal. Both live on the table component rather than here so the cell
  // renderers stay free of hook state.
  onShowItems: (run: ContainerBackupRun) => void;
  onShowError: (run: ContainerBackupRun) => void;
};

export function useBackupRunColumns({
  envNameById,
  onRestore,
  onDelete,
  onDownload,
  onOpenContainer,
  onShowItems,
  onShowError,
}: RunColumnArgs): ColumnDef<BackupRunRow, unknown>[] {
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const formatPrefs = useLocaleFormat();
  const { data: pluginAdapters } = useStorageAdapters();
  const sourceLabel = useSourceLabel();
  const group = t('backups.runs.columnsGroup');

  return useMemo(
    () => [
      {
        id: 'startedAt',
        accessorFn: (run) => formatTimestamp(run.startedAt, formatPrefs),
        header: t('backups.runs.table.startedAt'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">{formatTimestamp(row.original.startedAt, formatPrefs)}</span>
        ),
      },
      {
        id: 'duration',
        accessorFn: (run) => (run.durationMs > 0 ? `${Math.round(run.durationMs / 1000)}s` : DASH),
        header: t('backups.runs.table.duration'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
      },
      {
        id: 'source',
        accessorFn: (run) => sourceLabel(run),
        header: t('backups.runs.table.source'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <RunSourceCell row={row.original} label={sourceLabel(row.original)} />,
      },
      {
        id: 'container',
        accessorFn: (run) => run.subjectName,
        header: t('backups.runs.table.subject'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) =>
          isContainerRun(row.original) ? (
            <RunContainerCell run={row.original} onOpenContainer={onOpenContainer} />
          ) : (
            <span className="font-medium text-foreground">{row.original.subjectName}</span>
          ),
      },
      {
        id: 'environment',
        accessorFn: (run) =>
          environmentName(run.environmentId, envNameById.get(run.environmentId) ?? run.environmentId.slice(0, 8)),
        header: t('backups.runs.table.environment'),
      },
      {
        id: 'status',
        accessorFn: (run) => t(`backups.status.${run.status}`),
        header: t('backups.runs.table.status'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <RunStatusCell run={row.original} onShowError={onShowError} />,
      },
      {
        id: 'type',
        accessorFn: (run) => [t(runTypeKey(run)), run.providerKind].filter(Boolean).join(' '),
        header: t('backups.runs.table.type'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <RunTypeCell run={row.original} onShowItems={onShowItems} />,
      },
      {
        id: 'destinations',
        accessorFn: (run) => (run.destinations ?? []).map((d) => d.storageLocationName || '').join(' '),
        header: t('backups.runs.table.destinations'),
        enableSorting: false,
        cell: ({ row }) => <RunDestinationsCell run={row.original} pluginAdapters={pluginAdapters} />,
      },
      {
        id: 'size',
        accessorFn: (run) => formatBytes(run.archiveSize),
        header: t('backups.runs.table.size'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{formatBytes(row.original.archiveSize)}</span>
        ),
      },
      textColumn<BackupRunRow>('operation', t('backups.runs.table.operation'), (r) => r.operation, {
        group,
        width: 'sm',
      }),
      textColumn<BackupRunRow>('plan', t('backups.runs.table.plan'), (r) => r.planName, { group }),
      dateColumn<BackupRunRow>('completedAt', t('backups.runs.table.completedAt'), (r) => r.completedAt, { group }),
      dateColumn<BackupRunRow>('createdAt', t('backups.runs.table.createdAt'), (r) => r.createdAt, { group }),
      textColumn<BackupRunRow>('progressStage', t('backups.runs.table.progressStage'), (r) => r.progressStage, {
        group,
      }),
      boolColumn<BackupRunRow>(
        'containerIdStale',
        t('backups.runs.table.containerIdStale'),
        (r) => r.containerIdStale,
        { group },
      ),
      {
        id: 'actions',
        header: tc('labels.actions'),
        enableSorting: false,
        meta: { width: 'lg', locked: true, noExport: true, label: tc('labels.actions') } satisfies TableColumnMeta,
        cell: ({ row }) =>
          isContainerRun(row.original) ? (
            <RunActionsCell
              run={row.original}
              onRestore={onRestore}
              onDownload={onDownload}
              onDelete={onDelete}
              onOpenContainer={onOpenContainer}
            />
          ) : (
            <PluginRunActionsCell run={row.original} />
          ),
      },
    ],
    [
      envNameById,
      formatPrefs,
      onDelete,
      onDownload,
      onOpenContainer,
      onRestore,
      onShowError,
      onShowItems,
      pluginAdapters,
      sourceLabel,
      t,
      tc,
      group,
    ],
  );
}
