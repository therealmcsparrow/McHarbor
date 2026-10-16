// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconTrash, IconEdit, IconLock, IconEye } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { Role } from '../hooks/useRoles';
import { PERM } from '@core/auth/permissionKeys';

type RoleColumnActions = {
  onView: (role: Role) => void;
  onEdit: (role: Role) => void;
  onDelete: (roleId: string) => void;
};

/** Column definitions for the roles table. */
export function useRoleColumns({ onView, onEdit, onDelete }: RoleColumnActions) {
  const { t } = useTranslation('security');
  const group = t('columns.groupRole');
  const usage = t('columns.groupUsage');

  return useMemo<ColumnDef<Role, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (role) => role.name,
        header: t('roles.name'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate font-medium">{row.original.name}</div>
            {row.original.description && (
              <div className="truncate text-xs text-muted-foreground">{row.original.description}</div>
            )}
          </div>
        ),
      },
      {
        id: 'type',
        accessorFn: (role) => (role.isSystem ? t('roles.system') : t('roles.custom')),
        header: t('roles.type'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.isSystem ? (
            <Badge variant="secondary">
              <IconLock className="mr-1 size-3" />
              {t('roles.system')}
            </Badge>
          ) : (
            <Badge variant="outline">{t('roles.custom')}</Badge>
          ),
      },
      {
        id: 'permissions',
        // Sorted by count; a role with `*` sorts last, which is where the
        // one role that can do everything belongs.
        accessorFn: (role) => (role.permissions.includes('*') ? Number.MAX_SAFE_INTEGER : role.permissions.length),
        header: t('roles.permissions'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant="secondary">
            {row.original.permissions.includes('*')
              ? t('roles.fullAccess')
              : t('roles.permissionCount', { count: row.original.permissions.length })}
          </Badge>
        ),
      },
      textColumn<Role>('roleId', t('columns.id'), (r) => r.id, { group }),
      textColumn<Role>('description', t('columns.description'), (r) => r.description, { group, width: 'xl' }),
      dateColumn<Role>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<Role>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
      textColumn<Role>('userCount', t('columns.userCount'), (r) => r.userCount, { group: usage, width: 'xs' }),
      textColumn<Role>('groupCount', t('columns.groupCount'), (r) => r.groupCount, { group: usage, width: 'xs' }),
      textColumn<Role>('apiKeyCount', t('columns.apiKeyCount'), (r) => r.apiKeyCount, { group: usage, width: 'xs' }),
      {
        id: 'actions',
        header: t('roles.actions'),
        enableSorting: false,
        meta: { width: 'sm', locked: true, noExport: true, label: t('roles.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const role = row.original;
          return (
            <div className="flex justify-end gap-1">
              {role.isSystem ? (
                <Button variant="ghost" size="icon-sm" onClick={() => onView(role)} aria-label={t('roles.viewRole')}>
                  <IconEye className="size-3.5" />
                </Button>
              ) : (
                <>
                  <Button
                    permission={PERM.ROLES_MANAGE}
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onEdit(role)}
                    aria-label={t('roles.editRole')}
                  >
                    <IconEdit className="size-3.5" />
                  </Button>
                  <Button
                    permission={PERM.ROLES_MANAGE}
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => onDelete(role.id)}
                    aria-label={t('roles.deleteRole')}
                  >
                    <IconTrash className="size-3.5 text-destructive" />
                  </Button>
                </>
              )}
            </div>
          );
        },
      },
    ],
    [t, group, usage, onView, onEdit, onDelete],
  );
}
