// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { StatusError } from '@resources/components/ui/StatusError';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { type DesiredStateSource, useDeleteSource, useSources, useSyncSource } from '../hooks/useSources';
import { CreateSourceDialog } from '../components/CreateSourceDialog';

export default function SourcesPage() {
  const { t } = useTranslation('reconciler');
  const sources = useSources();
  const sync = useSyncSource();
  const remove = useDeleteSource();
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<DesiredStateSource | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const columns = useMemo<ColumnDef<DesiredStateSource, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('sources.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      {
        id: 'where',
        header: t('sources.where'),
        meta: { sortValue: (s: DesiredStateSource) => `${s.branch}:/${s.path}` },
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.branch}:/{row.original.path}
          </span>
        ),
      },
      {
        id: 'state',
        header: t('sources.state'),
        meta: { sortValue: (s: DesiredStateSource) => (s.lastError ? 'failing' : s.enabled ? 'syncing' : 'paused') },
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <Badge variant={row.original.lastError ? 'destructive' : row.original.enabled ? 'success' : 'secondary'}>
              {t(
                row.original.lastError
                  ? 'sources.failing'
                  : row.original.enabled
                    ? 'sources.syncing'
                    : 'sources.paused',
              )}
            </Badge>
            <StatusError error={row.original.lastError} title={t('sources.errorTitle')} />
          </span>
        ),
      },
      { accessorKey: 'rowCount', header: t('sources.rows') },
      {
        accessorKey: 'lastCommit',
        header: t('sources.commit'),
        cell: ({ row }) => <span className="font-mono text-xs">{row.original.lastCommit.slice(0, 10) || '—'}</span>,
      },
      {
        accessorKey: 'lastSyncedAt',
        header: t('sources.synced'),
        cell: ({ row }) => (row.original.lastSyncedAt ? timeAgo(row.original.lastSyncedAt) : '—'),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('reconciler.columnActions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('reconciler.columnActions', { ns: 'common' }) },
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon"
              permission={PERM.RECONCILER_MANAGE}
              aria-label={t('sources.syncNow')}
              title={t('sources.syncNow')}
              disabled={sync.isPending}
              onClick={() => sync.mutate(row.original.id)}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              permission={PERM.RECONCILER_MANAGE}
              aria-label={t('actions.delete', { ns: 'common' })}
              title={t('actions.delete', { ns: 'common' })}
              onClick={() => setDeleting(row.original)}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [t, sync],
  );

  return (
    <>
      <TablePage
        id="reconciler-sources"
        title={t('sources.title')}
        description={t('sources.description')}
        actions={
          <div className="flex items-center gap-2">
            <Button asChild variant="outline">
              <Link to="/reconciler">
                <IconArrowLeft className="h-4 w-4" /> {t('report.back')}
              </Link>
            </Button>
            <Button permission={PERM.RECONCILER_MANAGE} onClick={() => setCreating(true)}>
              <IconPlus className="h-4 w-4" /> {t('sources.add')}
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('sources.name'),
          fields: (s) => [s.name, s.branch, s.path],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void sources.refetch(), isFetching: sources.isFetching },
          export: { filename: 'mcharbor-desired-state-sources' },
        }}
        columns={columns}
        data={sources.data ?? []}
        loading={sources.isLoading}
        empty={{ title: t('sources.empty') }}
        getRowId={(s) => s.id}
      />
      {creating && <CreateSourceDialog onOpenChange={setCreating} />}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(isOpen) => {
          if (isOpen) return;
          setDeleting(null);
          remove.reset();
        }}
        title={t('sources.deleteTitle')}
        description={t('sources.deleteDescription', { name: deleting?.name ?? '' })}
        confirmLabel={t('sources.deleteConfirm')}
        loading={remove.isPending}
        permission={PERM.RECONCILER_MANAGE}
        onConfirm={() =>
          deleting && !remove.isPending && remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) })
        }
      >
        {remove.error && <Alert variant="destructive">{remove.error.message}</Alert>}
      </ConfirmDialog>
    </>
  );
}
