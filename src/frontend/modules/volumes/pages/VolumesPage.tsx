// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import {
  IconTrash,
  IconPlus,
  IconLayoutGrid,
  IconLayoutList,
  IconFilterOff,
} from '@tabler/icons-react';
import type { VolumeInfo } from '@core/types/docker';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid, type BatchAction } from '@resources/components/DataGrid';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';
import { useVolumes, useCreateVolume, useRemoveVolume, usePruneVolumes } from '../hooks/useVolumes';
import { useVolumesViewStore } from '../stores/volumes-view';
import { CreateVolumeDialog } from '../components/CreateVolumeDialog';
import { VolumeCardGrid } from '../components/VolumeCardGrid';

export default function VolumesPage() {
  const { t } = useTranslation('volumes');
  const { t: tc } = useTranslation('common');
  const { data: volumes = [], isLoading } = useVolumes();
  const createVolume = useCreateVolume();
  const removeVolume = useRemoveVolume();
  const pruneVolumes = usePruneVolumes();
  const { viewMode, setViewMode } = useVolumesViewStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);

  const columns = useMemo<ColumnDef<VolumeInfo, unknown>[]>(
    () => [
      {
        accessorKey: 'Name',
        header: t('columns.name'),
        cell: ({ row }) => <span className="font-medium">{row.original.Name}</span>,
      },
      { accessorKey: 'Driver', header: t('columns.driver') },
      {
        id: 'usage',
        header: t('columns.usage'),
        cell: ({ row }) => {
          const count = row.original.RefCount;
          return count > 0 ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="success">{t('badges.inUse')}</Badge>
              </TooltipTrigger>
              <TooltipContent>
                {count} {count === 1 ? 'container' : 'containers'}
              </TooltipContent>
            </Tooltip>
          ) : (
            <Badge variant="warning">{t('badges.unused')}</Badge>
          );
        },
      },
      {
        accessorKey: 'Mountpoint',
        header: t('columns.mountpoint'),
        cell: ({ row }) => (
          <span className="max-w-xs truncate font-mono text-xs text-muted-foreground">
            {row.original.Mountpoint}
          </span>
        ),
      },
      {
        accessorKey: 'CreatedAt',
        header: t('columns.created'),
        cell: ({ row }) => formatDate(row.original.CreatedAt),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        cell: ({ row }) => (
          <div className="flex items-center justify-end">
            <Button
              variant="ghost"
              size="icon"
              title={tc('actions.remove')}
              aria-label={tc('actions.remove')}
              onClick={(event) => {
                event.stopPropagation();
                setConfirmTarget(row.original.Name);
              }}
            >
              <IconTrash className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        ),
      },
    ],
    [t, tc]
  );

  const batchActions = useMemo<BatchAction[]>(
    () => [
      {
        label: tc('batch.remove'),
        icon: IconTrash,
        variant: 'destructive',
        confirm: true,
        onClick: (rows) => {
          for (const row of rows as VolumeInfo[]) {
            removeVolume.mutate(row.Name);
          }
        },
      },
    ],
    [tc, removeVolume]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description', { count: volumes.length })}
        actions={
          <>
            <Button onClick={() => setCreateOpen(true)}>
              <IconPlus className="h-4 w-4" /> {t('create.title')}
            </Button>
            <Button variant="outline" onClick={() => setPruneConfirmOpen(true)}>
              <IconFilterOff className="h-4 w-4" /> {t('pruneUnused')}
            </Button>
            <div className="h-6 w-px bg-border" />
            <div className="flex items-center rounded-lg border border-border">
              <Button
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('table')}
                aria-label={t('tableView')}
              >
                <IconLayoutList className="h-4 w-4" />
              </Button>
              <Button
                variant={viewMode === 'card' ? 'default' : 'ghost'}
                size="icon-sm"
                onClick={() => setViewMode('card')}
                aria-label={t('cardView')}
              >
                <IconLayoutGrid className="h-4 w-4" />
              </Button>
            </div>
          </>
        }
      />

      {viewMode === 'table' ? (
        <DataGrid
          data={volumes}
          columns={columns}
          searchKey="Name"
          searchPlaceholder={t('searchPlaceholder')}
          loading={isLoading}
          emptyMessage={t('emptyMessage')}
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.Name}
        />
      ) : (
        <VolumeCardGrid
          volumes={volumes}
          isLoading={isLoading}
          onRemove={setConfirmTarget}
        />
      )}

      <CreateVolumeDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        createVolume={createVolume}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) removeVolume.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={removeVolume.isPending}
      />

      <ConfirmDialog
        open={pruneConfirmOpen}
        onOpenChange={setPruneConfirmOpen}
        title={t('pruneUnused')}
        description={t('pruneDescription')}
        confirmLabel={t('pruneUnused')}
        onConfirm={() => {
          pruneVolumes.mutate();
          setPruneConfirmOpen(false);
        }}
        loading={pruneVolumes.isPending}
      />
    </div>
  );
}
