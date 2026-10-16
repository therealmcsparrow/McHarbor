// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
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
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { useCreateUser } from '@resources/hooks/useUsers';
import { useRoles } from '../hooks/useRoles';
import { PERM } from '@core/auth/permissionKeys';
import { usePasswordPolicy } from '@core/auth/usePasswordPolicy';
import { isPasswordValid } from '@core/auth/passwordPolicy';

type CreateUserDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateUserDialog({ open: isOpen, onOpenChange }: CreateUserDialogProps) {
  const { t } = useTranslation('security');
  const createUser = useCreateUser();
  const { data: roles } = useRoles();
  const { policy } = usePasswordPolicy();
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [roleId, setRoleId] = useState('');
  const [isActive, setIsActive] = useState(true);

  const roleOptions = useMemo(() => (roles ?? []).map((role) => ({ value: role.id, label: role.name })), [roles]);

  const [seeded, setSeeded] = useState<{ isOpen: boolean; roles: typeof roles }>({ isOpen: false, roles: undefined });
  if (seeded.isOpen !== isOpen || seeded.roles !== roles) {
    setSeeded({ isOpen, roles });
    if (isOpen) {
      const defaultRole = roles?.find((role) => role.id === 'role_viewer') ?? roles?.[0];
      setRoleId(defaultRole?.id ?? '');
    }
  }

  function resetForm() {
    setUsername('');
    setDisplayName('');
    setEmail('');
    setPassword('');
    setIsActive(true);
  }

  function handleSubmit() {
    createUser.mutate(
      {
        username,
        password,
        displayName: displayName || undefined,
        email: email || undefined,
        roleId: roleId || undefined,
        isActive,
      },
      {
        onSuccess: () => {
          resetForm();
          onOpenChange(false);
        },
      },
    );
  }

  // The same policy the backend enforces, fetched at runtime — a
  // hardcoded minimum here would let the user submit a password the
  // server rejects the moment an admin tightens the policy.
  const canSubmit = username.trim() !== '' && isPasswordValid(password, policy) && roleId !== '';

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
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
              aria-describedby="create-user-password-requirements"
            />
            <PasswordRequirements
              id="create-user-password-requirements"
              showHistory={false}
              password={password}
              policy={policy}
            />
          </div>

          <div className="grid gap-2">
            <label className="text-sm font-medium text-foreground">{t('users.defaultRole')}</label>
            <Select value={roleId} onChange={setRoleId} options={roleOptions} placeholder={t('users.selectRole')} />
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
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.USERS_MANAGE} onClick={handleSubmit} disabled={!canSubmit || createUser.isPending}>
            {createUser.isPending ? t('actions.processing', { ns: 'common' }) : t('actions.create', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
