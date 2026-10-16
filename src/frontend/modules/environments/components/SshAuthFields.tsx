// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';

interface SshAuthFieldsProps {
  sshAuthType: 'key' | 'password';
  onSSHAuthTypeChange?: (value: 'key' | 'password') => void;
  sshKey: string;
  onSSHKeyChange?: (value: string) => void;
  sshPassword: string;
  onSSHPasswordChange?: (value: string) => void;
  sshDeployAs: 'docker' | 'binary';
  onSSHDeployAsChange?: (value: 'docker' | 'binary') => void;
}

export function SshAuthFields({
  sshAuthType,
  onSSHAuthTypeChange,
  sshKey,
  onSSHKeyChange,
  sshPassword,
  onSSHPasswordChange,
  sshDeployAs,
  onSSHDeployAsChange,
}: SshAuthFieldsProps) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div>
        <Label className="mb-2">{t('create.sshAuthType')}</Label>
        <Select
          value={sshAuthType}
          onChange={(v) => onSSHAuthTypeChange?.(v as 'key' | 'password')}
          options={[
            { value: 'key', label: t('create.sshAuthKey') },
            { value: 'password', label: t('create.sshAuthPassword') },
          ]}
        />
      </div>
      {sshAuthType === 'key' ? (
        <div>
          <Label className="mb-2">{t('create.sshKey')}</Label>
          <textarea
            className="flex min-h-[100px] w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
            value={sshKey}
            onChange={(e) => onSSHKeyChange?.(e.target.value)}
            placeholder={t('create.sshKeyPlaceholder')}
            rows={4}
          />
        </div>
      ) : (
        <div>
          <Label className="mb-2">{t('create.sshPassword')}</Label>
          <Input
            type="password"
            value={sshPassword}
            onChange={(e) => onSSHPasswordChange?.(e.target.value)}
            placeholder="••••••••"
          />
          <p className="mt-1 text-xs text-warning">{t('create.sshPasswordWarning')}</p>
        </div>
      )}
      <div>
        <Label className="mb-2">{t('create.deployAs')}</Label>
        <Select
          value={sshDeployAs}
          onChange={(v) => onSSHDeployAsChange?.(v as 'docker' | 'binary')}
          options={[
            { value: 'docker', label: t('create.deployAsDocker') },
            { value: 'binary', label: t('create.deployAsBinary') },
          ]}
        />
      </div>
    </>
  );
}
