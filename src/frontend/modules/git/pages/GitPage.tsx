// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconGitBranch, IconRefresh, IconTrash, IconPlus } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { timeAgo } from '@resources/utils/format';
import {
  type GitRepo,
  useGitRepos,
  useSyncRepo,
  useCreateRepo,
  useRemoveRepo,
  STATUS_VARIANT,
} from '@resources/hooks/useGitRepos';
import { textColumn, dateColumn, boolColumn } from '@resources/layout/table-page/optional-columns';
import { CreateRepoDialog } from '../components/CreateRepoDialog';
import { PERM } from '@core/auth/permissionKeys';

export default function GitPage() {
  const { t } = useTranslation('common');
  const group = t('git.columnsGroup');
  const { data: repos = [], isLoading, isFetching, refetch } = useGitRepos();
  const syncRepo = useSyncRepo(t);
  const createRepo = useCreateRepo(t);
  const removeRepo = useRemoveRepo(t);

  const statusLabel = useCallback(
    (repo: GitRepo): string => {
      if (repo.lastSyncError) return t('git.statusError');
      if (repo.lastSyncAt) return t('git.statusSynced');
      return t('git.statusPending');
    },
    [t],
  );

  const [createOpen, setCreateOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  const columns = useMemo<ColumnDef<GitRepo, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('git.columnName'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <IconGitBranch className="h-4 w-4 text-muted-foreground" />
            <span className="font-medium">{row.original.name}</span>
          </div>
        ),
      },
      {
        accessorKey: 'url',
        header: t('git.columnUrl'),
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.url}</span>,
      },
      { accessorKey: 'branch', header: t('git.columnBranch') },
      {
        id: 'status',
        header: t('git.columnStatus'),
        meta: { sortValue: statusLabel },
        cell: ({ row }) => <Badge variant={STATUS_VARIANT(row.original)}>{statusLabel(row.original)}</Badge>,
      },
      {
        id: 'lastSynced',
        header: t('git.columnLastSynced'),
        meta: { sortValue: (r: GitRepo) => (r.lastSyncAt ? Date.parse(r.lastSyncAt) : null) },
        cell: ({ row }) => (row.original.lastSyncAt ? timeAgo(row.original.lastSyncAt) : t('git.never')),
      },
      boolColumn<GitRepo>('autoSync', t('git.columnAutoSync'), (r) => r.autoSync, { group }),
      textColumn<GitRepo>('syncInterval', t('git.columnSyncInterval'), (r) => r.syncInterval, { group, width: 'sm' }),
      textColumn<GitRepo>('authType', t('git.columnAuthType'), (r) => r.authType, { group, width: 'sm' }),
      dateColumn<GitRepo>('createdAt', t('git.columnCreatedAt'), (r) => r.createdAt, { group }),
      dateColumn<GitRepo>('updatedAt', t('git.columnUpdatedAt'), (r) => r.updatedAt, { group }),
      textColumn<GitRepo>('environment', t('git.columnEnvironment'), (r) => r.environmentName, {
        group: t('git.columnsGroupEnvironment'),
      }),
      textColumn<GitRepo>('lastDeployStatus', t('git.columnLastDeployStatus'), (r) => r.lastDeployStatus, {
        group: t('git.columnsGroupDeployment'),
      }),
      textColumn<GitRepo>(
        'lastDeployCommit',
        t('git.columnLastDeployCommit'),
        (r) => r.lastDeployCommit?.slice(0, 12),
        { group: t('git.columnsGroupDeployment') },
      ),
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('git.columnActions')}</span>,
        meta: { locked: true, noExport: true, label: t('git.columnActions') },
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              permission={PERM.GIT_MANAGE}
              variant="ghost"
              size="icon"
              title={t('git.syncTooltip')}
              aria-label={t('git.syncTooltip')}
              onClick={(eventItem) => {
                eventItem.stopPropagation();
                syncRepo.mutate(row.original.id);
              }}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
            <Button
              permission={PERM.GIT_MANAGE}
              variant="ghost"
              size="icon"
              title={t('git.removeTooltip')}
              aria-label={t('git.removeTooltip')}
              onClick={(evt) => {
                evt.stopPropagation();
                setConfirmTarget(row.original.id);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [syncRepo, t, statusLabel, group],
  );

  return (
    <>
      <TablePage
        id="git"
        title={t('git.title')}
        description={t('git.description')}
        actions={
          <Button permission={PERM.GIT_MANAGE} onClick={() => setCreateOpen(true)}>
            <IconPlus className="h-4 w-4" /> {t('git.addRepository')}
          </Button>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('git.searchPlaceholder'),
          fields: (repo) => [repo.name, repo.url, repo.branch, repo.lastSyncError],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-git-repositories' },
        }}
        columns={columns}
        data={repos}
        loading={isLoading}
        empty={{ title: t('git.noRepositories') }}
        getRowId={(repo) => repo.id}
      />

      <CreateRepoDialog open={createOpen} onOpenChange={setCreateOpen} createRepo={createRepo} />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setConfirmTarget(null)}
        title={t('git.removeTitle')}
        description={t('git.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) removeRepo.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={removeRepo.isPending}
      />
    </>
  );
}
