// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { TablePage } from '@resources/layout/TablePage';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { CreatePayloadDialog } from '../components/CreatePayloadDialog';
import { SwarmGuard } from '../components/SwarmGuard';
import { useCreateSwarmSecret, useRemoveSwarmSecret, useSwarmSecrets } from '../hooks/useSwarmSecrets';
import { formatDateTime } from '../date-format';
import type { SwarmSecret } from '../types';

export default function SwarmSecretsPage() {
  return (
    <SwarmGuard>
      <SwarmSecrets />
    </SwarmGuard>
  );
}

/**
 * The secrets list.
 *
 * There is no reveal affordance anywhere on this page — not a disabled one,
 * not a hidden one. Docker's secret API is write-only for the value, so a
 * control that appeared to offer it would be promising something the server
 * cannot do.
 */
function SwarmSecrets() {
  const { t } = useTranslation('swarm');
  const { data: secrets = [], isLoading, isFetching, refetch } = useSwarmSecrets();
  const create = useCreateSwarmSecret();
  const remove = useRemoveSwarmSecret();

  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [createOpen, setCreateOpen] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<SwarmSecret | null>(null);

  const columns = useMemo<ColumnDef<SwarmSecret, unknown>[]>(
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
        id: 'inUseBy',
        header: t('columns.inUseBy'),
        meta: { flex: true, sortValue: (secret: SwarmSecret) => secret.inUseBy.join(', ') },
        cell: ({ row }) =>
          row.original.inUseBy.length > 0 ? (
            <span className="truncate text-sm text-muted-foreground">{row.original.inUseBy.join(', ')}</span>
          ) : (
            <span className="text-xs text-muted-foreground">{t('secrets.unused')}</span>
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
        size: 60,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.remove')}
              permission={PERM.SWARM_SECRETS_DELETE}
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
        id="swarm-secrets"
        title={t('secrets.title')}
        description={t('secrets.description', { count: secrets.length })}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('secrets.searchPlaceholder'),
          fields: (s: SwarmSecret) => [s.name, s.stackName, ...s.inUseBy],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.SWARM_SECRETS_MANAGE} onClick={() => setCreateOpen(true)}>
              <IconPlus className="size-4" />
              {t('secrets.create')}
            </Button>
          ),
        }}
        columns={columns}
        data={secrets}
        loading={isLoading}
        empty={{ title: t('secrets.empty') }}
        getRowId={(row) => row.id}
      />

      <CreatePayloadDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={t('dialogs.createSecret.title')}
        description={t('dialogs.createSecret.description')}
        permission={PERM.SWARM_SECRETS_MANAGE}
        pending={create.isPending}
        onSubmit={(itemName, data) =>
          create.mutate({ name: itemName, data }, { onSuccess: () => setCreateOpen(false) })
        }
      />

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(nextOpen) => !nextOpen && setRemoveTarget(null)}
        title={t('dialogs.removeSecret.title')}
        description={
          removeTarget && removeTarget.inUseBy.length > 0
            ? t('dialogs.removeSecret.inUse', {
                name: removeTarget.name,
                services: removeTarget.inUseBy.join(', '),
              })
            : t('dialogs.removeSecret.description', { name: removeTarget?.name ?? '' })
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
