// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconEye, IconPlus, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { TablePage } from '@resources/layout/TablePage';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { ConfigViewerDialog } from '../components/ConfigViewerDialog';
import { CreatePayloadDialog } from '../components/CreatePayloadDialog';
import { SwarmGuard } from '../components/SwarmGuard';
import { useCreateSwarmConfig, useRemoveSwarmConfig, useSwarmConfigs } from '../hooks/useSwarmSecrets';
import { formatDateTime } from '../date-format';
import { formatBytes } from '../format';
import type { SwarmConfig } from '../types';

export default function SwarmConfigsPage() {
  return (
    <SwarmGuard>
      <SwarmConfigs />
    </SwarmGuard>
  );
}

function SwarmConfigs() {
  const { t } = useTranslation('swarm');
  const { data: configs = [], isLoading, isFetching, refetch } = useSwarmConfigs();
  const create = useCreateSwarmConfig();
  const remove = useRemoveSwarmConfig();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [createOpen, setCreateOpen] = useState(false);
  const [viewTarget, setViewTarget] = useState<SwarmConfig | null>(null);
  const [removeTarget, setRemoveTarget] = useState<SwarmConfig | null>(null);

  const columns = useMemo<ColumnDef<SwarmConfig, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        meta: { flex: true },
        cell: ({ row }) => <span className="truncate font-medium">{row.original.name}</span>,
      },
      {
        accessorKey: 'stackName',
        header: t('columns.stack'),
        size: 140,
        cell: ({ row }) =>
          row.original.stackName ? (
            <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
              {row.original.stackName}
            </Badge>
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          ),
      },
      {
        accessorKey: 'size',
        header: t('columns.size'),
        size: 90,
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{formatBytes(row.original.size)}</span>,
      },
      {
        id: 'inUseBy',
        header: t('columns.inUseBy'),
        meta: { flex: true, sortValue: (config: SwarmConfig) => config.inUseBy.join(', ') },
        cell: ({ row }) =>
          row.original.inUseBy.length > 0 ? (
            <span className="truncate text-sm text-muted-foreground">{row.original.inUseBy.join(', ')}</span>
          ) : (
            <span className="text-xs text-muted-foreground">{t('configs.unused')}</span>
          ),
      },
      {
        accessorKey: 'createdAt',
        header: t('columns.created'),
        size: 170,
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{formatDateTime(row.original.createdAt)}</span>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        size: 100,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1">
            {/* Unlike a secret, a config's payload is readable — the API
                returns it on inspect — so the viewer is a real affordance. */}
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.view')}
              permission={PERM.SWARM_CONFIGS_VIEW}
              onClick={(e) => {
                e.stopPropagation();
                setViewTarget(row.original);
              }}
            >
              <IconEye className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.remove')}
              permission={PERM.SWARM_CONFIGS_DELETE}
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
        id="swarm-configs"
        title={t('configs.title')}
        description={t('configs.description', { count: configs.length })}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('configs.searchPlaceholder'),
          fields: (c: SwarmConfig) => [c.name, c.stackName, ...c.inUseBy],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.SWARM_CONFIGS_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="size-4" />
              {t('configs.create')}
            </Button>
          ),
        }}
        columns={columns}
        data={configs}
        loading={isLoading}
        empty={{ title: t('configs.empty') }}
        getRowId={(row) => row.id}
        onRowClick={(row) => setViewTarget(row)}
      />

      <CreatePayloadDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={t('dialogs.createConfig.title')}
        description={t('dialogs.createConfig.description')}
        permission={PERM.SWARM_CONFIGS_MANAGE}
        pending={create.isPending}
        onSubmit={(itemName, data) =>
          create.mutate({ name: itemName, data }, { onSuccess: () => setCreateOpen(false) })
        }
      />

      <ConfigViewerDialog config={viewTarget} onOpenChange={() => setViewTarget(null)} />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('dialogs.removeConfig.title')}
        description={
          removeTarget && removeTarget.inUseBy.length > 0
            ? t('dialogs.removeConfig.inUse', {
                name: removeTarget.name,
                services: removeTarget.inUseBy.join(', '),
              })
            : t('dialogs.removeConfig.description', { name: removeTarget?.name ?? '' })
        }
        loading={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.id);
          setRemoveTarget(null);
        }}
      />
    </>
  );
}
