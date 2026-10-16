// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The group directory.
//
// This used to be a one-third list with a two-thirds panel beside it, so
// the member list and the role list shared what was left after the list
// took its column. The panel is now `/security/groups/:id`, built on the
// user detail page's shape, and the table gets the whole width — the same
// move the users tab made.
//
// What stays here is what a directory is for: find the group, see how many
// people are in it, and act on it without leaving the list.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLock, IconPlus } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { TablePage } from '@resources/layout/TablePage';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { textColumn, boolColumn, dateColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import { useDeleteGroup, useGroups, type Group } from '../hooks/useGroups';
import { GroupFormDialog } from './GroupFormDialog';
import { GroupRowActions } from './GroupRowActions';

export function GroupsTab() {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const navigate = useNavigate();
  const { data: groups = [], isLoading, isFetching, refetch } = useGroups();
  const deleteGroup = useDeleteGroup();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [createOpen, setCreateOpen] = useState(false);
  // The dialogs the action column opens live here rather than in the cell,
  // so a row unmounting mid-refetch cannot take an open dialog with it.
  const [editGroup, setEditGroup] = useState<Group | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Group | null>(null);
  const groupLabel = t('columns.groupGroups');
  const access = t('columns.groupAccess');

  const columns = useMemo<ColumnDef<Group, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (group) => group.name,
        header: t('groups.name'),
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-1.5">
            {row.original.isSystem && (
              <>
                <IconLock aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="sr-only">{t('groups.system')}</span>
              </>
            )}
            <span className="truncate font-medium">{row.original.name}</span>
          </div>
        ),
      },
      {
        id: 'description',
        accessorFn: (group) => group.description || '-',
        header: t('groups.groupDescription'),
        cell: ({ row }) => (
          <span className="block truncate text-muted-foreground">{row.original.description || '—'}</span>
        ),
      },
      {
        id: 'members',
        accessorFn: (group) => group.memberCount,
        header: t('groups.members'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant="secondary" className="tabular-nums">
            {row.original.memberCount}
          </Badge>
        ),
      },
      {
        id: 'createdAt',
        accessorFn: (group) => timeAgo(group.createdAt),
        header: t('groups.createdAt'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      textColumn<Group>('groupId', t('columns.id'), (r) => r.id, { group: groupLabel }),
      boolColumn<Group>('isSystem', t('columns.isSystem'), (r) => r.isSystem, { group: groupLabel }),
      dateColumn<Group>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group: groupLabel }),
      listColumn<Group>('roles', t('columns.roles'), (r) => r.roleNames, { group: access }),
      {
        id: 'actions',
        header: t('groups.actions'),
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true, label: t('groups.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => <GroupRowActions group={row.original} onEdit={setEditGroup} onDelete={setDeleteTarget} />,
      },
    ],
    [t, groupLabel, access],
  );

  return (
    <>
      <TablePage
        id="groups"
        // Rendered inside the security settings tabs.
        header={false}
        title={t('groups.description')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('groups.name'),
          fields: (group) => [group.name, group.description],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.GROUPS_MANAGE} size="sm" onClick={() => setCreateOpen(true)}>
              <IconPlus className="mr-1 size-3.5" />
              {tc('actions.create')}
            </Button>
          ),
        }}
        columns={columns}
        data={groups}
        loading={isLoading}
        empty={{ title: t('groups.noGroups') }}
        getRowId={(group) => group.id}
        onRowClick={(group) => navigate(`/security/groups/${group.id}`)}
      />

      <GroupFormDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editGroup && (
        <GroupFormDialog open onOpenChange={(openState) => !openState && setEditGroup(null)} group={editGroup} />
      )}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(isOpen) => !isOpen && setDeleteTarget(null)}
        title={t('groups.deleteGroup')}
        description={t('groups.deleteGroupConfirm')}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteGroup.mutate(deleteTarget.id, {
            onSuccess: () => setDeleteTarget(null),
          });
        }}
        loading={deleteGroup.isPending}
      />
    </>
  );
}
