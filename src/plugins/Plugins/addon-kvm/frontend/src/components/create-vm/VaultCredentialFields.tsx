// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useVaultSecrets, type VaultSecret } from '../../hooks/useVaultSecrets';
import { SECRETS_PERM } from '../../permissions';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';

/** The slice of the wizard's form the vault fields read and write. */
export type VaultCredentialForm = Pick<
  CreateVMForm,
  | 'environmentId'
  | 'password'
  | 'setPassword'
  | 'passwordFromVault'
  | 'setPasswordFromVault'
  | 'passwordSecret'
  | 'setPasswordSecret'
  | 'sshKeySecrets'
  | 'setSshKeySecrets'
>;

function VaultCopyNote() {
  const { t } = useTranslation('vm');
  return <p className="mt-1 text-xs text-muted-foreground">{t('create.setup.vaultCopyNote')}</p>;
}

const asOptions = (secrets: VaultSecret[]) =>
  secrets.map((secret) => ({ value: secret.name, label: secret.name, description: secret.kind }));

/** The cloud-init password: typed, or taken by name from the vault. */
export function PasswordField({ form }: { form: VaultCredentialForm }) {
  const { t } = useTranslation('vm');
  const gate = usePermissionGate(SECRETS_PERM.VIEW);
  const {
    environmentId,
    password,
    setPassword,
    passwordFromVault,
    setPasswordFromVault,
    passwordSecret,
    setPasswordSecret,
  } = form;
  const { data: secrets = [] } = useVaultSecrets(environmentId, gate.allowed && passwordFromVault);

  return (
    <div>
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor={passwordFromVault ? 'vm-password-secret' : 'vm-password'}>{t('create.setup.password')}</Label>
        <div className="flex items-center gap-2" title={gate.allowed ? undefined : gate.reason}>
          <Label htmlFor="vm-password-vault" className="text-xs font-normal text-muted-foreground">
            {t('create.setup.passwordFromVault')}
          </Label>
          <Switch
            id="vm-password-vault"
            checked={passwordFromVault}
            disabled={!gate.allowed}
            onCheckedChange={setPasswordFromVault}
          />
        </div>
      </div>
      {passwordFromVault ? (
        <Select
          value={passwordSecret}
          onChange={setPasswordSecret}
          options={asOptions(secrets.filter((s) => s.kind !== 'ssh_key'))}
          placeholder={t('create.setup.secretPlaceholder')}
          ariaLabel={t('create.setup.passwordSecret')}
        />
      ) : (
        <Input
          id="vm-password"
          type="password"
          value={password}
          onChange={(evt) => setPassword(evt.target.value)}
          autoComplete="new-password"
        />
      )}
      {!gate.allowed && <p className="mt-1 text-xs text-muted-foreground">{gate.reason}</p>}
      {passwordFromVault && <VaultCopyNote />}
    </div>
  );
}

/** SSH public keys taken by name from the vault, beside the typed ones. */
export function VaultKeysField({ form }: { form: VaultCredentialForm }) {
  const { t } = useTranslation('vm');
  const gate = usePermissionGate(SECRETS_PERM.VIEW);
  const { environmentId, sshKeySecrets, setSshKeySecrets } = form;
  const { data: secrets = [] } = useVaultSecrets(environmentId, gate.allowed);

  return (
    <div title={gate.allowed ? undefined : gate.reason}>
      <Label>{t('create.setup.sshKeySecrets')}</Label>
      <MultiSelect
        value={sshKeySecrets}
        onChange={setSshKeySecrets}
        options={asOptions(secrets.filter((s) => s.kind === 'ssh_key' || !s.kind))}
        placeholder={t('create.setup.secretsPlaceholder')}
        ariaLabel={t('create.setup.sshKeySecrets')}
        disabled={!gate.allowed}
        searchable
      />
      {gate.allowed ? <VaultCopyNote /> : <p className="mt-1 text-xs text-muted-foreground">{gate.reason}</p>}
    </div>
  );
}
