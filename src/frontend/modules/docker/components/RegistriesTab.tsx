// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconPlugConnected, IconPencil, IconTrash, IconCheck, IconX } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { useRegistries, useTestRegistry, type Registry } from '../hooks/useRegistries';
import { PERM } from '@core/auth/permissionKeys';
import { useRegistryOptionalColumns } from './registryOptionalColumns';
import { AddRegistryDialog, EditRegistryDialog, DeleteRegistryDialog } from './RegistryDialogs';

export function RegistriesTab() {
  const { t } = useTranslation('docker');
  const { data, isLoading, isFetching, refetch } = useRegistries();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [addOpen, setAddOpen] = useState(false);
  const [editReg, setEditReg] = useState<Registry | null>(null);
  const [deleteReg, setDeleteReg] = useState<Registry | null>(null);

  const registries = data?.items ?? [];
  const optionalColumns = useRegistryOptionalColumns();

  const columns = useMemo<ColumnDef<Registry, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (reg) => reg.name,
        header: t('registries.columnName'),
        cell: ({ row }) => <span className="font-medium text-foreground">{row.original.name}</span>,
      },
      {
        id: 'url',
        accessorFn: (reg) => reg.url,
        header: t('registries.columnUrl'),
        cell: ({ row }) => <code className="text-xs">{row.original.url}</code>,
      },
      {
        id: 'username',
        accessorFn: (reg) => reg.username || '-',
        header: t('registries.columnUsername'),
      },
      {
        id: 'default',
        accessorFn: (reg) => (reg.isDefault ? t('registries.columnDefault') : ''),
        header: t('registries.columnDefault'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.isDefault ? <Badge variant="default">{t('registries.columnDefault')}</Badge> : null,
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: t('registries.columnActions'),
        enableSorting: false,
        meta: {
          width: 'md',
          locked: true,
          noExport: true,
          label: t('registries.columnActions'),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <RegistryActionsCell
            registry={row.original}
            onEdit={() => setEditReg(row.original)}
            onDelete={() => setDeleteReg(row.original)}
          />
        ),
      },
    ],
    [t, optionalColumns],
  );

  return (
    <>
      <TablePage
        id="registries"
        // Rendered inside the Docker settings tabs.
        header={false}
        title={t('registries.title')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('registries.title'),
          fields: (reg) => [reg.name, reg.url, reg.username],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          // No export: this is a list of registry endpoints and the accounts
          // used against them, which is not something to produce as a file.
          actions: (
            <Button permission={PERM.REGISTRY_MANAGE} size="sm" onClick={() => setAddOpen(true)}>
              <IconPlus className="mr-1.5 size-4" />
              {t('registries.add')}
            </Button>
          ),
        }}
        columns={columns}
        data={registries}
        loading={isLoading}
        empty={{ title: t('registries.noRegistries') }}
        getRowId={(reg) => reg.id}
      />

      <AddRegistryDialog open={addOpen} onOpenChange={setAddOpen} />
      {editReg && (
        <EditRegistryDialog
          open={!!editReg}
          onOpenChange={(next) => {
            if (!next) setEditReg(null);
          }}
          registry={editReg}
        />
      )}
      {deleteReg && (
        <DeleteRegistryDialog
          open={!!deleteReg}
          onOpenChange={(next) => {
            if (!next) setDeleteReg(null);
          }}
          registry={deleteReg}
        />
      )}
    </>
  );
}

function RegistryActionsCell({
  registry,
  onEdit,
  onDelete,
}: {
  registry: Registry;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation('docker');
  const testMutation = useTestRegistry();

  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        permission={PERM.REGISTRY_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('registries.testTooltip')}
        onClick={() => testMutation.mutate(registry.id)}
        disabled={testMutation.isPending}
      >
        {testMutation.isPending ? (
          <Spinner size="sm" />
        ) : testMutation.isSuccess ? (
          <IconCheck className="size-4 text-success" />
        ) : testMutation.isError ? (
          <IconX className="size-4 text-destructive" />
        ) : (
          <IconPlugConnected className="size-4" />
        )}
      </Button>
      <Button
        permission={PERM.REGISTRY_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('registries.editTooltip')}
        onClick={onEdit}
      >
        <IconPencil className="size-4" />
      </Button>
      <Button
        permission={PERM.REGISTRY_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('registries.removeTooltip')}
        onClick={onDelete}
      >
        <IconTrash className="size-4" />
      </Button>
    </div>
  );
}
