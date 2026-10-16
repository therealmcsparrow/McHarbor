// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import { IconDeviceFloppy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { usePasswordPolicy } from '@core/auth/usePasswordPolicy';
import { isPasswordValid } from '@core/auth/passwordPolicy';
import { MFASettingsPanel } from '../MFASettingsPanel';
import { SessionsManager } from '@resources/components/SessionsManager';
import { useChangeOwnPassword } from '../../hooks/useChangeOwnPassword';

type SecurityTabProps = {
  /**
   * Identity-provider accounts have no local password to change — their
   * IdP owns the credential.
   */
  identityProviderLocked: boolean;
};

export function SecurityTab({ identityProviderLocked }: SecurityTabProps) {
  const { t } = useTranslation('common');

  // Self-service password change. The backend enforces the runtime
  // password policy; we mirror it client-side so the requirements
  // resolve while typing instead of arriving as a 400. The mutation is
  // fired from the local user only — admins force-reset other users
  // via the admin account page.
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmNewPassword, setConfirmNewPassword] = useState('');
  // Server-side rejections the browser cannot predict — most often
  // "this password was used recently", which needs the password history.
  const [serverError, setServerError] = useState('');
  const changeOwnPassword = useChangeOwnPassword();
  const { policy } = usePasswordPolicy();

  const passwordMismatch = newPassword !== confirmNewPassword && confirmNewPassword !== '';
  const policyMet = isPasswordValid(newPassword, policy);
  const canChangePassword = !identityProviderLocked && currentPassword.length > 0 && policyMet && !passwordMismatch;

  async function handleChangePassword(evt2: FormEvent<HTMLFormElement>) {
    evt2.preventDefault();
    if (!canChangePassword || changeOwnPassword.isPending) return;
    setServerError('');
    try {
      await changeOwnPassword.mutateAsync({ currentPassword, newPassword });
      toast.success(t('profile.account.password.changeSuccess'));
      setCurrentPassword('');
      setNewPassword('');
      setConfirmNewPassword('');
    } catch (err) {
      // The backend already localises policy rejections (too short,
      // missing a character class, reused). Surfacing its message beats
      // a generic "change failed" the user cannot act on.
      const msg = err instanceof Error && err.message ? err.message : t('profile.account.password.changeFailed');
      setServerError(msg);
      toastError(err, t('profile.account.password.changeFailed'));
    }
  }

  return (
    <div className="space-y-6">
      {/* Your own sessions. `userId={null}` selects the self path, which reads
          /auth/sessions and revokes through /auth/sessions/* - no admin
          permission involved, because revoking your own session is account
          self-management. The admin view of someone else's sessions lives on
          Security -> Users. */}
      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.security.sessionsTitle')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.security.sessionsDescription')}</p>
        </div>
        <SessionsManager userId={null} />
      </section>
      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.account.password.title')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.security.passwordDescription')}</p>
        </div>

        {identityProviderLocked ? (
          <p
            className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning"
            data-testid="profile-password-idp-locked"
          >
            {t('profile.account.password.idpLockedDescription')}
          </p>
        ) : (
          <form className="space-y-4" onSubmit={handleChangePassword}>
            <div className="grid gap-4 sm:grid-cols-3">
              <div>
                <Label htmlFor="profile-current-password" className="mb-1.5 text-xs">
                  {t('profile.account.password.current')}
                </Label>
                <Input
                  id="profile-current-password"
                  type="password"
                  autoComplete="current-password"
                  variant="outline"
                  value={currentPassword}
                  onChange={(entry) => setCurrentPassword(entry.target.value)}
                  required
                />
              </div>
              <div>
                <Label htmlFor="profile-new-password" className="mb-1.5 text-xs">
                  {t('profile.account.password.new')}
                </Label>
                <Input
                  id="profile-new-password"
                  type="password"
                  autoComplete="new-password"
                  variant="outline"
                  value={newPassword}
                  onChange={(eventItem) => setNewPassword(eventItem.target.value)}
                  aria-describedby="profile-password-requirements"
                  required
                />
              </div>
              <div>
                <Label htmlFor="profile-confirm-password" className="mb-1.5 text-xs">
                  {t('profile.account.password.confirm')}
                </Label>
                <Input
                  id="profile-confirm-password"
                  type="password"
                  autoComplete="new-password"
                  variant="outline"
                  value={confirmNewPassword}
                  onChange={(evt) => setConfirmNewPassword(evt.target.value)}
                  required
                />
              </div>
            </div>
            <PasswordRequirements id="profile-password-requirements" password={newPassword} policy={policy} />
            {passwordMismatch && <p className="text-xs text-destructive">{t('profile.account.password.mismatch')}</p>}
            {serverError && (
              <p role="alert" className="text-xs text-destructive">
                {serverError}
              </p>
            )}
            <Button type="submit" variant="default" disabled={!canChangePassword || changeOwnPassword.isPending}>
              <IconDeviceFloppy className="size-4" />
              {changeOwnPassword.isPending ? t('actions.saving') : t('profile.account.password.submit')}
            </Button>
          </form>
        )}
      </section>

      <MFASettingsPanel className="mc-glass rounded-lg p-6" />
    </div>
  );
}
