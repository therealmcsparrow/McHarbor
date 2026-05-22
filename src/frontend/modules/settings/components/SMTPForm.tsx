// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';

export type SMTPFormData = {
  name: string;
  host: string;
  port: number;
  encryption: string;
  authMethod: string;
  username: string;
  password: string;
  fromAddress: string;
  fromName: string;
};

type SMTPFormProps = {
  data: SMTPFormData;
  onChange: (data: SMTPFormData) => void;
  isEdit?: boolean;
};

const ENCRYPTION_OPTIONS = [
  { value: 'none', label: 'None' },
  { value: 'starttls', label: 'STARTTLS' },
  { value: 'ssl_tls', label: 'SSL/TLS' },
];

const AUTH_OPTIONS = [
  { value: 'none', label: 'None' },
  { value: 'plain', label: 'PLAIN' },
  { value: 'login', label: 'LOGIN' },
  { value: 'cram_md5', label: 'CRAM-MD5' },
];

export function SMTPForm({ data, onChange, isEdit }: SMTPFormProps) {
  const { t } = useTranslation('settings');
  const needsCreds = data.authMethod !== 'none' && data.authMethod !== '';

  function update(partial: Partial<SMTPFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="smtp-name" className="mb-1">{t('email.nameLabel')}</Label>
        <Input
          variant="outline"
          id="smtp-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('email.namePlaceholder')}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="smtp-host" className="mb-1">{t('email.hostLabel')}</Label>
          <Input
            variant="outline"
            id="smtp-host"
            value={data.host}
            onChange={(e) => update({ host: e.target.value })}
            placeholder={t('email.hostPlaceholder')}
          />
        </div>
        <div>
          <Label htmlFor="smtp-port" className="mb-1">{t('email.portLabel')}</Label>
          <Input
            variant="outline"
            id="smtp-port"
            type="number"
            value={data.port || ''}
            onChange={(e) => update({ port: parseInt(e.target.value) || 0 })}
            placeholder={t('email.portPlaceholder')}
          />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label className="mb-1">{t('email.encryptionLabel')}</Label>
          <Select
            variant="outline"
            value={data.encryption}
            onChange={(val) => update({ encryption: val })}
            options={ENCRYPTION_OPTIONS}
            searchable={false}
          />
        </div>
        <div>
          <Label className="mb-1">{t('email.authMethodLabel')}</Label>
          <Select
            variant="outline"
            value={data.authMethod}
            onChange={(val) => update({ authMethod: val })}
            options={AUTH_OPTIONS}
            searchable={false}
          />
        </div>
      </div>

      {needsCreds && (
        <div className="grid grid-cols-2 gap-3">
          <div>
            <Label htmlFor="smtp-username" className="mb-1">{t('email.usernameLabel')}</Label>
            <Input
              variant="outline"
              id="smtp-username"
              value={data.username}
              onChange={(e) => update({ username: e.target.value })}
              placeholder={t('email.usernamePlaceholder')}
            />
          </div>
          <div>
            <Label htmlFor="smtp-password" className="mb-1">{t('email.passwordLabel')}</Label>
            <Input
              variant="outline"
              id="smtp-password"
              type="password"
              value={data.password}
              onChange={(e) => update({ password: e.target.value })}
              placeholder={isEdit ? t('email.passwordHint') : t('email.passwordPlaceholder')}
            />
          </div>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="smtp-from-address" className="mb-1">{t('email.fromAddressLabel')}</Label>
          <Input
            variant="outline"
            id="smtp-from-address"
            type="email"
            value={data.fromAddress}
            onChange={(e) => update({ fromAddress: e.target.value })}
            placeholder={t('email.fromAddressPlaceholder')}
          />
        </div>
        <div>
          <Label htmlFor="smtp-from-name" className="mb-1">{t('email.fromNameLabel')}</Label>
          <Input
            variant="outline"
            id="smtp-from-name"
            value={data.fromName}
            onChange={(e) => update({ fromName: e.target.value })}
            placeholder={t('email.fromNamePlaceholder')}
          />
        </div>
      </div>
    </div>
  );
}
