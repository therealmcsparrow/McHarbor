// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@tanstack/react-table';
import { IconRefresh, IconSearch, IconAlertTriangle, IconPlus, IconTrash } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { DataGrid } from '@resources/components/DataGrid';
import { StatusBadge, RECONCILER_STATUS } from '@resources/components/ui/StatusBadge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { timeAgo } from '@resources/utils/format';
import {
  type DesiredState,
  useDesiredStates,
  useReconcile,
  useCheckDrift,
  useCreateDesiredState,
  useDeleteDesiredState,
  deriveStatus,
} from '../hooks/useReconciler';
import { CreateDesiredStateDialog } from '../components/CreateDesiredStateDialog';

export default function ReconcilerPage() {
  const { t } = useTranslation('common');
  const { data: states = [], isLoading } = useDesiredStates();
  const reconcile = useReconcile(t);
  const checkDrift = useCheckDrift(t);
  const createState = useCreateDesiredState(t);
  const deleteState = useDeleteDesiredState(t);

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  const columns = useMemo<ColumnDef<DesiredState, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('reconciler.columnName'),
        cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
      },
      {
        accessorKey: 'containerName',
        header: t('reconciler.columnContainer'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.containerName}</span>
        ),
      },
      {
        accessorKey: 'imageRef',
        header: t('reconciler.columnImage'),
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.imageRef}</span>
        ),
      },
      {
        id: 'status',
        header: t('reconciler.columnStatus'),
        cell: ({ row }) => (
          <StatusBadge status={deriveStatus(row.original)} map={RECONCILER_STATUS} />
        ),
      },
      {
        id: 'lastReconciled',
        header: t('reconciler.columnLastReconciled'),
        cell: ({ row }) =>
          row.original.lastReconcile ? timeAgo(row.original.lastReconcile) : t('reconciler.never'),
      },
      {
        id: 'driftDetected',
        header: t('reconciler.columnDrift'),
        cell: ({ row }) =>
          row.original.driftDetected ? (
            <div className="flex items-center gap-1 text-yellow-500">
              <IconAlertTriangle className="h-4 w-4" /> {t('reconciler.driftYes')}
            </div>
          ) : (
            <span className="text-muted-foreground">{t('reconciler.driftNo')}</span>
          ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('reconciler.columnActions')}</span>,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            <Button
              variant="ghost"
              size="icon"
              title={t('reconciler.reconcileTooltip')}
              aria-label={t('reconciler.reconcileTooltip')}
              onClick={(event) => {
                event.stopPropagation();
                reconcile.mutate(row.original.id);
              }}
            >
              <IconRefresh className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={t('reconciler.checkDriftTooltip')}
              aria-label={t('reconciler.checkDriftTooltip')}
              onClick={(event) => {
                event.stopPropagation();
                checkDrift.mutate(row.original.id);
              }}
            >
              <IconSearch className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={t('reconciler.deleteTooltip')}
              aria-label={t('reconciler.deleteTooltip')}
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
    [reconcile, checkDrift, t]
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('reconciler.title')}
        description={t('reconciler.description')}
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <IconPlus className="h-4 w-4" /> {t('reconciler.addDesiredState')}
          </Button>
        }
      />

      <DataGrid
        data={states}
        columns={columns}
        searchKey="name"
        searchPlaceholder={t('reconciler.searchPlaceholder')}
        loading={isLoading}
        emptyMessage={t('reconciler.noDesiredStates')}
      />

      <CreateDesiredStateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        createState={createState}
      />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('reconciler.deleteTitle')}
        description={t('reconciler.deleteDescription')}
        confirmLabel={t('actions.delete')}
        onConfirm={() => {
          if (confirmTarget) deleteState.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={deleteState.isPending}
      />
    </div>
  );
}

