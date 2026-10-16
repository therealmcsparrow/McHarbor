// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { usePluginNavigate, usePluginSearchParams } from '../nav-context';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { TablePage } from '@resources/layout/TablePage';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { DeployStackDialog } from '../components/DeployStackDialog';
import { StatusBadge } from '../components/StatusBadge';
import { SwarmGuard } from '../components/SwarmGuard';
import { useRemoveSwarmStack, useSwarmStacks } from '../hooks/useSwarmStacks';
import { formatDateTime } from '../date-format';
import type { SwarmStack } from '../types';

export default function SwarmStacksPage() {
  return (
    <SwarmGuard>
      <SwarmStacks />
    </SwarmGuard>
  );
}

function SwarmStacks() {
  const { t } = useTranslation('swarm');
  const navigate = usePluginNavigate();
  const { data: stacks = [], isLoading, isFetching, refetch } = useSwarmStacks();
  const remove = useRemoveSwarmStack();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [deployOpen, setDeployOpen] = useState(false);

  // The Ctrl+K palette's deploy action lands here as `?action=deploy`; the dialog is open
  // for as long as the param is, and closing it drops the param.
  const [searchParams, setSearchParams] = usePluginSearchParams();
  const canDeploy = usePermissionGate(PERM.SWARM_STACKS_MANAGE).allowed;
  const deployRequested = searchParams.get('action') === 'deploy' && canDeploy;
  const [removeTarget, setRemoveTarget] = useState<SwarmStack | null>(null);

  const columns = useMemo<ColumnDef<SwarmStack, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => <span className="truncate font-medium">{row.original.name}</span>,
      },
      {
        accessorKey: 'services',
        header: t('columns.services'),
        size: 100,
        cell: ({ row }) => <span className="text-sm">{row.original.services}</span>,
      },
      {
        id: 'tasks',
        header: t('columns.tasks'),
        size: 110,
        meta: { sortValue: (stack: SwarmStack) => stack.runningTasks },
        cell: ({ row }) => {
          const { runningTasks, desiredTasks } = row.original;
          return (
            <StatusBadge tone={runningTasks >= desiredTasks ? 'success' : 'warning'}>
              {runningTasks}/{desiredTasks}
            </StatusBadge>
          );
        },
      },
      {
        id: 'networks',
        header: t('columns.networks'),
        meta: { flex: true, sortValue: (stack: SwarmStack) => stack.networks.join(', ') },
        cell: ({ row }) =>
          row.original.networks.length > 0 ? (
            <span className="truncate text-sm text-muted-foreground">{row.original.networks.join(', ')}</span>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          ),
      },
      {
        accessorKey: 'updatedAt',
        header: t('columns.updated'),
        size: 170,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{formatDateTime(row.original.updatedAt)}</span>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        size: 60,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.remove')}
              permission={PERM.SWARM_STACKS_DELETE}
              onClick={(e) => {
                e.stopPropagation();
                setRemoveTarget(row.original);
              }}
            >
              <IconTrash className="size-4" />
            </Button>
          </div>
        ),
      },
    ],
    [t],
  );

  return (
    <>
      <TablePage
        id="swarm-stacks"
        title={t('stacks.title')}
        description={t('stacks.description', { count: stacks.length })}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('stacks.searchPlaceholder'),
          fields: (s: SwarmStack) => [s.name, ...s.networks],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.SWARM_STACKS_MANAGE} onClick={() => setDeployOpen(true)}>
              <IconPlus className="size-4" />
              {t('stacks.deploy')}
            </Button>
          ),
        }}
        columns={columns}
        data={stacks}
        loading={isLoading}
        empty={{ title: t('stacks.empty') }}
        getRowId={(row) => row.name}
        // A stack has no page of its own; its services are the thing to look
        // at, and the services list already carries the stack chip.
        onRowClick={(row) => navigate(`services?stack=${encodeURIComponent(row.name)}`)}
      />

      <DeployStackDialog
        open={deployOpen || deployRequested}
        onOpenChange={(nextOpen) => {
          setDeployOpen(nextOpen);
          if (!nextOpen && deployRequested) {
            setSearchParams((current) => {
              current.delete('action');
              return current;
            });
          }
        }}
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('dialogs.removeStack.title')}
        description={t('dialogs.removeStack.description', {
          name: removeTarget?.name ?? '',
          services: removeTarget?.services ?? 0,
        })}
        loading={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.name);
          setRemoveTarget(null);
        }}
      />
    </>
  );
}
