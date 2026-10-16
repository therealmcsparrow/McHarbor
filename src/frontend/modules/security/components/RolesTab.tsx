// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconShieldCheck } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import { PolicyConflictWarnings } from './governance/PolicyConflictWarnings';
import type { SearchMode } from '@resources/utils/search-filter';
import { useRoles } from '../hooks/useRoles';
import { DeleteRoleDialog } from './DeleteRoleDialog';
import { RoleFormDialog, RoleViewDialog } from './RoleFormDialog';
import { DefaultRoleBanner } from './DefaultRoleBanner';
import { useRoleColumns } from './useRoleColumns';
import type { Role } from '../hooks/useRoles';
import { PERM } from '@core/auth/permissionKeys';

export function RolesTab() {
  const { t } = useTranslation('security');
  const { data: roles, isLoading, isFetching, refetch } = useRoles();
  const [editRole, setEditRole] = useState<Role | null>(null);
  const [viewRole, setViewRole] = useState<Role | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const onView = useCallback((role: Role) => setViewRole(role), []);
  const onEdit = useCallback((role: Role) => setEditRole(role), []);
  const onDelete = useCallback((id: string) => setDeleteId(id), []);
  const columns = useRoleColumns({ onView, onEdit, onDelete });

  return (
    <>
      <PolicyConflictWarnings subject="role" />
      <TablePage<Role>
        id="security-roles"
        // Rendered inside the security tabs, which already carry the title.
        header={false}
        title={t('roles.title')}
        banner={<DefaultRoleBanner roles={roles ?? []} />}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('roles.name'),
          fields: (role) => [role.name, role.description],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          actions: (
            <Button permission={PERM.ROLES_MANAGE} size="sm" onClick={() => setCreateOpen(true)}>
              <IconPlus className="mr-1 size-3.5" />
              {t('roles.createRole')}
            </Button>
          ),
        }}
        columns={columns}
        data={roles ?? []}
        loading={isLoading}
        empty={{
          icon: <IconShieldCheck className="size-10 text-muted-foreground" />,
          title: t('roles.noRoles'),
          description: t('roles.description'),
        }}
        getRowId={(role) => role.id}
      />

      <RoleFormDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editRole && (
        <RoleFormDialog
          open={!!editRole}
          onOpenChange={(dialogOpen) => {
            if (!dialogOpen) setEditRole(null);
          }}
          role={editRole}
        />
      )}

      <DeleteRoleDialog roleId={deleteId} onOpenChange={(openState) => !openState && setDeleteId(null)} />

      {viewRole && (
        <RoleViewDialog
          open={!!viewRole}
          onOpenChange={(isOpen) => {
            if (!isOpen) setViewRole(null);
          }}
          role={viewRole}
        />
      )}
    </>
  );
}
