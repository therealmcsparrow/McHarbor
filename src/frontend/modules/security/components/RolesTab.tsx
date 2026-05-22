// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash, IconEdit, IconLock, IconEye } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { useRoles, useDeleteRole } from '../hooks/useRoles';
import { RoleFormDialog, RoleViewDialog } from './RoleFormDialog';
import type { Role } from '../hooks/useRoles';

export function RolesTab() {
  const { t } = useTranslation('security');
  const { data: roles, isLoading } = useRoles();
  const [editRole, setEditRole] = useState<Role | null>(null);
  const [viewRole, setViewRole] = useState<Role | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const deleteRole = useDeleteRole();

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{t('roles.description')}</p>
        <Button variant="outline" size="sm" onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-1 size-3.5" />
          {t('roles.createRole')}
        </Button>
      </div>

      <div className="grid gap-3">
        {roles?.map((role) => (
          <div key={role.id} className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
            <div className="flex items-center gap-3">
              <div>
                <div className="flex items-center gap-2">
                  <span className="font-medium">{role.name}</span>
                  {role.isSystem ? (
                    <Badge variant="secondary">
                      <IconLock className="mr-1 size-3" />
                      {t('roles.system')}
                    </Badge>
                  ) : (
                    <Badge variant="outline">{t('roles.custom')}</Badge>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">{role.description}</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant="secondary">
                {role.permissions.includes('*')
                  ? t('roles.fullAccess')
                  : t('roles.permissionCount', { count: role.permissions.length })}
              </Badge>
              {role.isSystem ? (
                <Button variant="ghost" size="sm" onClick={() => setViewRole(role)} aria-label={t('roles.viewRole')}>
                  <IconEye className="size-3.5" />
                </Button>
              ) : (
                <>
                  <Button variant="ghost" size="sm" onClick={() => setEditRole(role)} aria-label={t('roles.editRole')}>
                    <IconEdit className="size-3.5" />
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setDeleteId(role.id)} aria-label={t('roles.deleteRole')}>
                    <IconTrash className="size-3.5" />
                  </Button>
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      <RoleFormDialog open={createOpen} onOpenChange={setCreateOpen} />
      {editRole && (
        <RoleFormDialog
          open={!!editRole}
          onOpenChange={(open) => { if (!open) setEditRole(null); }}
          role={editRole}
        />
      )}

      <ConfirmDialog
        open={!!deleteId}
        onOpenChange={() => setDeleteId(null)}
        title={t('roles.deleteRole')}
        description={t('roles.deleteRoleConfirm')}
        onConfirm={() => {
          if (deleteId) {
            deleteRole.mutate(deleteId, { onSuccess: () => setDeleteId(null) });
          }
        }}
        loading={deleteRole.isPending}
      />

      {viewRole && (
        <RoleViewDialog
          open={!!viewRole}
          onOpenChange={(open) => { if (!open) setViewRole(null); }}
          role={viewRole}
        />
      )}
    </div>
  );
}
