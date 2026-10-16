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
import { PERM } from '@core/auth/permissionKeys';
import { useCreateUser } from '@resources/hooks/useUsers';
import { useRoles } from '../hooks/useRoles';

type CreateServicePrincipalDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateServicePrincipalDialog({ open: isOpen, onOpenChange }: CreateServicePrincipalDialogProps) {
  const { t } = useTranslation('security');
  const createUser = useCreateUser();
  const resetMutation = createUser.reset;
  const { data: roles } = useRoles();
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [roleId, setRoleId] = useState('');

  const roleOptions = useMemo(() => (roles ?? []).map((role) => ({ value: role.id, label: role.name })), [roles]);

  const [wasOpen, setWasOpen] = useState(false);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (isOpen) {
      setUsername('');
      setDisplayName('');
      setRoleId('');
    }
  }
  useEffect(() => {
    if (isOpen) resetMutation();
  }, [isOpen, resetMutation]);

  function handleSubmit() {
    createUser.mutate(
      { username, displayName: displayName || undefined, roleId, isActive: true, kind: 'service' },
      { onSuccess: () => onOpenChange(false) },
    );
  }

  const canSubmit = username.trim() !== '' && roleId !== '';

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('users.createServicePrincipal')}</DialogTitle>
          <DialogDescription>{t('users.createServicePrincipalDescription')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <p className="rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
            {t('users.servicePrincipalNoSignIn')}
          </p>

          <div className="grid gap-2">
            <label htmlFor="service-principal-username" className="text-sm font-medium text-foreground">
              {t('users.username')}
            </label>
            <Input
              id="service-principal-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={t('users.username')}
            />
          </div>

          <div className="grid gap-2">
            <label htmlFor="service-principal-display-name" className="text-sm font-medium text-foreground">
              {t('users.displayName')}
            </label>
            <Input
              id="service-principal-display-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t('users.displayName')}
            />
          </div>

          <div className="grid gap-2">
            <span className="text-sm font-medium text-foreground">{t('users.role')}</span>
            <Select
              ariaLabel={t('users.role')}
              value={roleId}
              onChange={setRoleId}
              options={roleOptions}
              placeholder={t('users.selectRole')}
            />
            <p className="text-xs text-muted-foreground">{t('users.servicePrincipalRoleHint')}</p>
          </div>

          {createUser.isError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {createUser.error.message}
            </p>
          )}
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
