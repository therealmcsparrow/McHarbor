// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconGitBranch, IconRefresh, IconTrash, IconPlus } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
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
} from '../hooks/useGitRepos';
import { CreateRepoDialog } from '../components/CreateRepoDialog';

export default function GitPage() {
  const { t } = useTranslation('common');
  const { data: repos = [], isLoading } = useGitRepos();
  const syncRepo = useSyncRepo(t);
  const createRepo = useCreateRepo(t);
  const removeRepo = useRemoveRepo(t);

  const statusLabel = useCallback((repo: GitRepo): string => {
    if (repo.lastSyncError) return t('git.statusError');
    if (repo.lastSyncAt) return t('git.statusSynced');
    return t('git.statusPending');
  }, [t]);

  const [createOpen, setCreateOpen] = useState(false);
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
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.url}</span>
        ),
      },
      { accessorKey: 'branch', header: t('git.columnBranch') },
      {
        id: 'status',
        header: t('git.columnStatus'),
        cell: ({ row }) => (
          <Badge variant={STATUS_VARIANT(row.original)}>
            {statusLabel(row.original)}
          </Badge>
        ),
      },
      {
        id: 'lastSynced',
        header: t('git.columnLastSynced'),
        cell: ({ row }) =>
          row.original.lastSyncAt ? timeAgo(row.original.lastSyncAt) : t('git.never'),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('git.columnActions')}</span>,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon"
              title={t('git.syncTooltip')}
              aria-label={t('git.syncTooltip')}
              onClick={(event) => {
                event.stopPropagation();
                syncRepo.mutate(row.original.id);
              }}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={t('git.removeTooltip')}
              aria-label={t('git.removeTooltip')}
              onClick={(event) => {
                event.stopPropagation();
                setConfirmTarget(row.original.id);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [syncRepo, t, statusLabel]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('git.title')}
        description={t('git.description')}
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <IconPlus className="h-4 w-4" /> {t('git.addRepository')}
          </Button>
        }
      />

      <DataGrid
        data={repos}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('git.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('git.noRepositories')}
      />

      <CreateRepoDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        createRepo={createRepo}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('git.removeTitle')}
        description={t('git.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) removeRepo.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={removeRepo.isPending}
      />
    </div>
  );
}

