// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The backup runs and plans tables.
//
// Both used to be hand-written `<table>` markup with selection state owned by
// BackupsPage and a separate bulk-action toolbar rendered as a sibling. They are
// TablePage now: selection lives in the layout, and the bulk actions ride the
// bulk bar that swaps into the filter row. The `data-testid` hooks the old
// markup carried are preserved.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArchive, IconDownload, IconTrash } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import type { BatchAction } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { type ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import { BackupRunErrorDialog, BackupRunItemsDialog } from '../components/BackupRunDetailDialogs';
import { useBackupRunColumns } from './backup-columns';
import { isContainerRun, type BackupRunRow } from './backup-run-rows';
import { useSourceLabel } from '../hooks/useSourceLabel';
import { environmentName, runTypeKey } from './backups-page-helpers';

export { BackupPlansTable } from './backup-plans-table';
export { retentionText } from './backups-page-helpers';
export { StaleIdBadge } from '../components/StaleIdBadge';

export function BackupRunsTable({
  runs,
  loading,
  envNameById,
  onRestore,
  onDelete,
  onDownload,
  onOpenContainer,
  onBulkDownload,
  onBulkDelete,
}: {
  runs: BackupRunRow[];
  loading: boolean;
  envNameById: Map<string, string>;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onDownload: (run: ContainerBackupRun) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
  onBulkDownload: (runs: ContainerBackupRun[]) => void;
  onBulkDelete: (runs: ContainerBackupRun[]) => void;
}) {
  const { t } = useTranslation('containers');
  const sourceLabel = useSourceLabel();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  // The two info modals the type and status columns open. Held here rather
  // than in BackupsPage: nothing above this table needs them, and the column
  // factory must stay hook-free.
  const [itemsTarget, setItemsTarget] = useState<ContainerBackupRun | null>(null);
  const [errorTarget, setErrorTarget] = useState<ContainerBackupRun | null>(null);

  const columns = useBackupRunColumns({
    envNameById,
    onRestore,
    onDelete,
    onDownload,
    onOpenContainer,
    onShowItems: setItemsTarget,
    onShowError: setErrorTarget,
  });

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: t('backups.bulkDownload'),
        icon: IconDownload,
        onClick: (rows) => onBulkDownload((rows as BackupRunRow[]).filter(isContainerRun)),
      },
      {
        label: t('backups.bulkDelete'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => onBulkDelete((rows as BackupRunRow[]).filter(isContainerRun)),
      },
    ],
    [onBulkDelete, onBulkDownload, t],
  );

  const table = (
    <TablePage
      id="backup-runs"
      // Rendered inside the backups tab bar, which carries the heading.
      header={false}
      title={t('backups.tabRuns')}
      search={{
        value: query,
        onChange: setQuery,
        mode: searchMode,
        onModeChange: setSearchMode,
        placeholder: t('backups.runs.table.subject'),
        fields: (run) => [
          run.subjectName,
          run.containerId,
          run.status,
          sourceLabel(run),
          t(runTypeKey(run)),
          environmentName(run.environmentId, envNameById.get(run.environmentId) ?? ''),
        ],
      }}
      toolbar={{
        columns: true,
        density: true,
        export: { filename: 'mcharbor-backup-runs' },
      }}
      columns={columns}
      data={runs}
      loading={loading && runs.length === 0}
      empty={{
        icon: <IconArchive className="size-8" />,
        title: t('backups.runs.emptyTitle'),
        description: t('backups.runs.emptyHint'),
      }}
      selectable
      batchActions={batchActions}
      getRowId={(run) => run.id}
      getRowTestId={(run) => `run-row-${run.id}`}
    />
  );

  return (
    <>
      {table}
      <BackupRunItemsDialog run={itemsTarget} onClose={() => setItemsTarget(null)} />
      <BackupRunErrorDialog run={errorTarget} onClose={() => setErrorTarget(null)} />
    </>
  );
}
