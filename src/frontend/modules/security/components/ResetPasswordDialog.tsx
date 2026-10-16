// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
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
import { Switch } from '@resources/components/ui/Switch';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { useAdminSetPassword, useForceResetPassword } from '../hooks/useAccountManagement';
import { PERM } from '@core/auth/permissionKeys';
import { usePasswordPolicy } from '@core/auth/usePasswordPolicy';
import { isPasswordValid } from '@core/auth/passwordPolicy';

type ResetPasswordDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  userId: string;
  username: string;
};

/**
 * ResetPasswordDialog has two modes:
 *
 *   - "set" — admin types a new password directly (uses
 *     /users/{id}/password/admin). The user must use this password
 *     on their next login.
 *   - "force" — admin flips a switch; the user must choose their
 *     own new password on next login (uses
 *     /users/{id}/force-reset-password). The password is unchanged.
 *
 * Both modes use the same dialog to keep the UX consistent. The
 * caller picks the default mode via the `defaultMode` prop.
 */
export function ResetPasswordDialog({
  open: isOpen,
  onOpenChange,
  userId,
  username,
  defaultMode = 'force',
}: ResetPasswordDialogProps & { defaultMode?: 'set' | 'force' }) {
  const { t } = useTranslation('security');
  const [mode, setMode] = useState<'set' | 'force'>(defaultMode);
  const [password, setPassword] = useState('');
  const { policy } = usePasswordPolicy();

  const setPassword_ = useAdminSetPassword();
  const forceReset = useForceResetPassword();

  function handleSubmit() {
    if (mode === 'set') {
      setPassword_.mutate({ userId, newPassword: password }, { onSuccess: () => onOpenChange(false) });
    } else {
      forceReset.mutate(userId, {
        onSuccess: () => onOpenChange(false),
      });
    }
  }

  const isPending = setPassword_.isPending || forceReset.isPending;
  // In 'set' mode the admin types the password themselves, so it is
  // held to the same runtime policy as any other password.
  const canSubmit = mode === 'force' || isPasswordValid(password, policy);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('resetPassword.title')}</DialogTitle>
          <DialogDescription>
            {t('resetPassword.description', {
              username,
            })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <Flex align="center" justify="between" className="rounded-lg border border-border px-3 py-2">
            <div>
              <TypographyText size="sm" weight="medium">
                {t('resetPassword.forceLabel')}
              </TypographyText>
              <TypographyText size="xs" tone="muted">
                {t('resetPassword.forceHint')}
              </TypographyText>
            </div>
            <Switch
              checked={mode === 'force'}
              onCheckedChange={(checked) => setMode(checked ? 'force' : 'set')}
              aria-label={t('resetPassword.forceLabel')}
            />
          </Flex>

          {mode === 'set' && (
            <div className="grid gap-2">
              <label className="text-sm font-medium text-foreground">{t('resetPassword.newPassword')}</label>
              <Input
                autoComplete="new-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                aria-describedby="admin-set-password-requirements"
              />
              <PasswordRequirements id="admin-set-password-requirements" password={password} policy={policy} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.ACCOUNT_PASSWORD_RESET} onClick={handleSubmit} disabled={!canSubmit || isPending}>
            {isPending ? t('actions.processing', { ns: 'common' }) : t('actions.apply', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
