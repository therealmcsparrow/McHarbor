// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useCreateRole, useUpdateRole, useAvailablePermissions } from '../hooks/useRoles';
import { PermissionPicker } from './PermissionPicker';
import type { Role } from '../hooks/useRoles';

type RoleViewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role: Role;
};

export function RoleViewDialog({ open, onOpenChange, role }: RoleViewDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const { data: allPermissions } = useAvailablePermissions();
  const isFullAccess = role.permissions.includes('*');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {role.name}
            <Badge variant="secondary">
              <IconLock className="mr-1 size-3" />
              {t('roles.system')}
            </Badge>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {role.description && (
            <p className="text-sm text-muted-foreground">{role.description}</p>
          )}

          {isFullAccess ? (
            <div className="rounded-lg border border-primary/20 bg-primary/5 p-4 text-center">
              <p className="font-medium">{t('roles.fullAccess')}</p>
              <p className="mt-1 text-sm text-muted-foreground">{t('roles.fullAccessDescription')}</p>
            </div>
          ) : (
            <div>
              <label className="mb-2 block text-sm font-medium">{t('roles.permissions')}</label>
              <PermissionPicker
                allPermissions={allPermissions ?? []}
                selected={role.permissions}
                readOnly
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{tc('actions.close')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type RoleFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role?: Role;
};

export function RoleFormDialog({ open, onOpenChange, role }: RoleFormDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const { data: allPermissions } = useAvailablePermissions();
  const [name, setName] = useState(role?.name ?? '');
  const [description, setDescription] = useState(role?.description ?? '');
  const [permissions, setPermissions] = useState<string[]>(role?.permissions ?? []);
  const isEdit = !!role;

  const handleSubmit = () => {
    if (isEdit) {
      updateRole.mutate({ id: role.id, name, description, permissions }, {
        onSuccess: () => onOpenChange(false),
      });
    } else {
      createRole.mutate({ name, description, permissions }, {
        onSuccess: () => {
          onOpenChange(false);
          setName('');
          setDescription('');
          setPermissions([]);
        },
      });
    }
  };

  const isPending = createRole.isPending || updateRole.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? t('roles.editRole') : t('roles.createRole')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <Input placeholder={t('roles.name')} value={name} onChange={(e) => setName(e.target.value)} />
          <Input placeholder={t('roles.roleDescription')} value={description} onChange={(e) => setDescription(e.target.value)} />

          <div>
            <label className="mb-2 block text-sm font-medium">{t('roles.permissions')}</label>
            <PermissionPicker
              allPermissions={allPermissions ?? []}
              selected={permissions}
              onChange={setPermissions}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{tc('actions.cancel')}</Button>
          <Button onClick={handleSubmit} disabled={!name || isPending}>
            {isPending ? tc('actions.processing') : (isEdit ? tc('actions.save') : tc('actions.create'))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

