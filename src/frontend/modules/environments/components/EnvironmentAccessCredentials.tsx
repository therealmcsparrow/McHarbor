// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';

type EnvironmentAccessCredentialsProps = {
  authType: string;
  onAuthTypeChange: (value: string) => void;
  secretId: string;
  onSecretIdChange: (value: string) => void;
  secretOptions: Array<{ value: string; label: string }>;
  sshKey: string;
  onSshKeyChange: (value: string) => void;
  sshPassword: string;
  onSshPasswordChange: (value: string) => void;
  saving: boolean;
  canSave: boolean;
  onSave: () => void;
};

export function EnvironmentAccessCredentials({
  authType,
  onAuthTypeChange,
  secretId,
  onSecretIdChange,
  secretOptions,
  sshKey,
  onSshKeyChange,
  sshPassword,
  onSshPasswordChange,
  saving,
  canSave,
  onSave,
}: EnvironmentAccessCredentialsProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-medium text-foreground">{t('detail.access.credentialTitle')}</h3>

      <div className="space-y-4">
        <div className="max-w-xs">
          <Label className="mb-2">{t('detail.access.authType')}</Label>
          <Select
            value={authType}
            onChange={onAuthTypeChange}
            ariaLabel={t('detail.access.authType')}
            options={[
              { value: 'key', label: t('detail.access.authKey') },
              { value: 'password', label: t('detail.access.authPassword') },
            ]}
          />
        </div>

        <div>
          <Label className="mb-2">{t('detail.access.secret')}</Label>
          <Select
            value={secretId}
            onChange={onSecretIdChange}
            ariaLabel={t('detail.access.secret')}
            options={secretOptions}
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('detail.access.secretHelp')}</p>
        </div>

        {authType === 'key' ? (
          <div>
            <Label className="mb-2">{t('detail.access.newKey')}</Label>
            <textarea
              value={sshKey}
              onChange={(changeEvent) => onSshKeyChange(changeEvent.target.value)}
              rows={5}
              spellCheck={false}
              autoComplete="off"
              aria-label={t('detail.access.newKey')}
              className="w-full rounded-lg border border-border bg-card p-2 font-mono text-xs text-foreground"
            />
          </div>
        ) : (
          <div className="max-w-md">
            <Label className="mb-2">{t('detail.access.newPassword')}</Label>
            <Input
              type="password"
              value={sshPassword}
              autoComplete="new-password"
              onChange={(changeEvent) => onSshPasswordChange(changeEvent.target.value)}
            />
          </div>
        )}

        <Alert variant="info" description={t('detail.access.credentialHelp')} />

        <div className="flex justify-end">
          <Button permission={PERM.ENVIRONMENTS_MANAGE} disabled={saving || !canSave} onClick={onSave}>
            {saving ? <Spinner className="mr-1.5 h-4 w-4" /> : null}
            {t('actions.save', { ns: 'common' })}
          </Button>
        </div>
      </div>
    </div>
  );
}
