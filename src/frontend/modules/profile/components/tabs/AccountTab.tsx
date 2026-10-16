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
import { useAuth } from '@core/auth/useAuth';

const EMAIL_PLACEHOLDER = 'name@example.com';

type AccountTabProps = {
  username: string;
  displayName: string | null;
  email: string | null;
  /** False for the built-in `system` account. */
  canEdit: boolean;
  /**
   * Identity-provider users (OIDC / SAML) authenticate against an external
   * source of truth for displayName and email. Editing those fields in
   * McHarbor would create a divergence between what McHarbor shows and what
   * the IdP shows the next time the user logs in, so the inputs render
   * read-only and a banner explains why.
   */
  identityProviderLocked: boolean;
};

export function AccountTab({ username, displayName, email, canEdit, identityProviderLocked }: AccountTabProps) {
  const { t } = useTranslation('common');
  const updateProfile = useAuth((s) => s.updateProfile);
  const [profileDisplayName, setProfileDisplayName] = useState(displayName ?? '');
  const [profileEmail, setProfileEmail] = useState(email ?? '');
  const [saving, setSaving] = useState(false);

  const [seeded, setSeeded] = useState({ displayName, email });
  if (seeded.displayName !== displayName || seeded.email !== email) {
    setSeeded({ displayName, email });
    setProfileDisplayName(displayName ?? '');
    setProfileEmail(email ?? '');
  }

  const changed = profileDisplayName.trim() !== (displayName ?? '') || profileEmail.trim() !== (email ?? '');

  async function handleSubmit(entry: FormEvent<HTMLFormElement>) {
    entry.preventDefault();
    if (!canEdit || saving) return;

    setSaving(true);
    const result = await updateProfile({
      displayName: profileDisplayName.trim(),
      email: profileEmail.trim(),
    });
    setSaving(false);

    if (result.success) {
      toast.success(t('profile.account.toast.updated'));
      return;
    }
    toastError(result.error, t('profile.account.toast.updateFailed'));
  }

  return (
    <div className="space-y-6">
      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.account.editTitle')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.account.description')}</p>
        </div>

        <form className="space-y-4" onSubmit={handleSubmit}>
          {identityProviderLocked && (
            <div
              className="rounded-md border border-warning/30 bg-warning/10 p-3 text-xs text-warning"
              data-testid="profile-idp-locked-banner"
            >
              {t('profile.account.idpLockedDescription')}
            </div>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="profile-display-name" className="mb-1.5 text-xs">
                {t('profile.account.displayName')}
              </Label>
              <Input
                id="profile-display-name"
                value={profileDisplayName}
                onChange={(eventItem) => setProfileDisplayName(eventItem.target.value)}
                maxLength={120}
                disabled={!canEdit || saving || identityProviderLocked}
                readOnly={identityProviderLocked}
                placeholder={username}
                variant="outline"
              />
            </div>
            <div>
              <Label htmlFor="profile-email" className="mb-1.5 text-xs">
                {t('profile.account.email')}
              </Label>
              <Input
                id="profile-email"
                type="email"
                value={profileEmail}
                onChange={(evt) => setProfileEmail(evt.target.value)}
                maxLength={254}
                disabled={!canEdit || saving || identityProviderLocked}
                readOnly={identityProviderLocked}
                placeholder={EMAIL_PLACEHOLDER}
                variant="outline"
              />
            </div>
          </div>
          {!canEdit && <p className="text-xs text-muted-foreground">{t('profile.account.systemAccount')}</p>}
          <Button type="submit" variant="default" disabled={!canEdit || !changed || saving || identityProviderLocked}>
            <IconDeviceFloppy className="size-4" />
            {saving ? t('actions.saving') : t('profile.account.saveProfile')}
          </Button>
        </form>
      </section>
    </div>
  );
}
