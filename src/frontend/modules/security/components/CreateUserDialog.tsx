// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { useCreateUser } from '../hooks/useUsers';
import { useRoles } from '../hooks/useRoles';

type CreateUserDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateUserDialog({ open, onOpenChange }: CreateUserDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const createUser = useCreateUser();
  const { data: roles } = useRoles();
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [roleId, setRoleId] = useState('');
  const [isActive, setIsActive] = useState(true);

  const roleOptions = useMemo(
    () => (roles ?? []).map((role) => ({ value: role.id, label: role.name })),
    [roles],
  );

  useEffect(() => {
    if (!open) {
      return;
    }

    const defaultRole = roles?.find((role) => role.id === 'role_viewer') ?? roles?.[0];
    setRoleId(defaultRole?.id ?? '');
  }, [open, roles]);

  function resetForm() {
    setUsername('');
    setDisplayName('');
    setEmail('');
    setPassword('');
    setIsActive(true);
  }

  function handleSubmit() {
    createUser.mutate({
      username,
      password,
      displayName: displayName || undefined,
      email: email || undefined,
      roleId: roleId || undefined,
      isActive,
    }, {
      onSuccess: () => {
        resetForm();
        onOpenChange(false);
      },
    });
  }

  const canSubmit = username.trim() !== '' && password.length >= 8 && roleId !== '';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('users.createUser')}</DialogTitle>
          <DialogDescription>{t('users.createUserDescription')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.username')}</label>
            <Input
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t('users.username')}
            />
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.displayName')}</label>
            <Input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t('users.displayName')}
            />
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.email')}</label>
            <Input
              autoComplete="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder={t('users.email')}
            />
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.password')}</label>
            <Input
              autoComplete="new-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={t('users.password')}
            />
            <p className="text-xs text-muted-foreground">{t('users.passwordHint')}</p>
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.defaultRole')}</label>
            <Select
              value={roleId}
              onChange={setRoleId}
              options={roleOptions}
              placeholder={t('users.selectRole')}
            />
          </div>

          <div className="flex items-center justify-between rounded-lg border border-border px-3 py-2">
            <div>
              <p className="text-sm font-medium text-foreground">{t('users.active')}</p>
              <p className="text-xs text-muted-foreground">{t('users.activeDescription')}</p>
            </div>
            <Switch checked={isActive} onCheckedChange={setIsActive} aria-label={t('users.active')} />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{tc('actions.cancel')}</Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || createUser.isPending}>
            {createUser.isPending ? tc('actions.processing') : tc('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
