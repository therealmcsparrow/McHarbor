// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The user directory.
//
// This used to be a two-thirds table with a one-third rail beside it,
// which meant group assignments, role assignments, permission overrides,
// account actions and the session list all competed for a third of the
// width — every one of them a list that wanted the full page. The rail is
// now `/security/users/:id`, built on the container detail page's shape,
// and the table gets the whole width it always needed.
//
// What stays here is what a directory is for: find the account, see at a
// glance whether it can sign in, and act on it without leaving the list.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconRobot } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { TablePage } from '@resources/layout/TablePage';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import type { SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { textColumn, dateColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import { useUsers, type UserItem } from '@resources/hooks/useUsers';
import { CreateUserDialog } from './CreateUserDialog';
import { CreateServicePrincipalDialog } from './CreateServicePrincipalDialog';
import { UserNameCell } from './UserNameCell';
import { ResetPasswordDialog } from './ResetPasswordDialog';
import { DisableAccountDialog } from './DisableAccountDialog';
import { UserRowActions } from './UserRowActions';
import { userAccountState } from './user-status';

export function UsersTab() {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const navigate = useNavigate();
  const { data: users = [], isLoading, isFetching, refetch } = useUsers();
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [createOpen, setCreateOpen] = useState(false);
  const [serviceOpen, setServiceOpen] = useState(false);
  // The two dialogs the action column opens live here rather than in the
  // cell, so a row unmounting mid-refetch cannot take an open dialog with it.
  const [resetUser, setResetUser] = useState<UserItem | null>(null);
  const [disableUser, setDisableUser] = useState<UserItem | null>(null);
  const group = t('columns.groupUser');
  const access = t('columns.groupAccess');

  const columns = useMemo<ColumnDef<UserItem, unknown>[]>(
    () => [
      {
        id: 'username',
        accessorFn: (user) => user.displayName || user.username,
        header: t('users.username'),
        cell: ({ row }) => <UserNameCell user={row.original} />,
      },
      {
        id: 'email',
        accessorFn: (user) => user.email || '-',
        header: t('users.email'),
      },
      {
        id: 'status',
        accessorFn: (user) => {
          const state = userAccountState(user);
          if (state === 'disabled') return t('users.inactive');
          return state === 'locked' ? t('users.locked') : t('users.active');
        },
        header: t('users.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const state = userAccountState(row.original);
          if (state === 'disabled') {
            return <Badge variant="secondary">{t('users.inactive')}</Badge>;
          }
          if (state === 'locked') {
            return <Badge variant="destructive">{t('users.locked')}</Badge>;
          }
          return <Badge variant="success">{t('users.active')}</Badge>;
        },
      },
      {
        id: 'lastLogin',
        accessorFn: (user) => (user.lastLogin ? timeAgo(user.lastLogin) : t('users.never')),
        header: t('users.lastLogin'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      textColumn<UserItem>('userId', t('columns.id'), (r) => r.id, { group }),
      textColumn<UserItem>('usernameRaw', t('columns.usernameRaw'), (r) => r.username, { group }),
      textColumn<UserItem>('displayName', t('columns.displayName'), (r) => r.displayName, { group }),
      textColumn<UserItem>('legacyRole', t('columns.legacyRole'), (r) => r.role, { group }),
      dateColumn<UserItem>('lockedUntil', t('columns.lockedUntil'), (r) => r.lockedUntil, { group }),
      dateColumn<UserItem>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<UserItem>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
      listColumn<UserItem>('roles', t('columns.roles'), (r) => r.roleNames, { group: access }),
      listColumn<UserItem>('groups', t('columns.groups'), (r) => r.groupNames, { group: access }),
      {
        id: 'actions',
        header: t('users.actions'),
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true, label: t('users.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <UserRowActions user={row.original} onResetPassword={setResetUser} onDisable={setDisableUser} />
        ),
      },
    ],
    [t, group, access],
  );

  return (
    <>
      <TablePage
        id="users"
        // Rendered inside the security settings tabs.
        header={false}
        title={t('users.description')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('users.username'),
          fields: (user) => [user.username, user.displayName, user.email],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          // No export: a user directory with e-mail addresses is not
          // something to hand out as a downloadable file from a settings tab.
          actions: (
            <>
              <Button permission={PERM.USERS_MANAGE} size="sm" variant="outline" onClick={() => setServiceOpen(true)}>
                <IconRobot className="mr-1 size-3.5" aria-hidden="true" />
                {t('users.servicePrincipal')}
              </Button>
              <Button permission={PERM.USERS_MANAGE} size="sm" onClick={() => setCreateOpen(true)}>
                <IconPlus className="mr-1 size-3.5" />
                {tc('actions.create')}
              </Button>
            </>
          ),
        }}
        columns={columns}
        data={users}
        loading={isLoading}
        empty={{ title: t('users.noUsers') }}
        getRowId={(user) => user.id}
        onRowClick={(user) => navigate(`/security/users/${user.id}`)}
      />

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
      <CreateServicePrincipalDialog open={serviceOpen} onOpenChange={setServiceOpen} />
      {resetUser && (
        <ResetPasswordDialog
          open
          onOpenChange={(openState) => !openState && setResetUser(null)}
          userId={resetUser.id}
          username={resetUser.username}
        />
      )}
      {disableUser && (
        <DisableAccountDialog
          open
          onOpenChange={(isOpen) => !isOpen && setDisableUser(null)}
          userId={disableUser.id}
          username={disableUser.username}
        />
      )}
    </>
  );
}
