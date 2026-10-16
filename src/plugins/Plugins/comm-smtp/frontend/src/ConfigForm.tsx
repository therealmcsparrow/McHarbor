// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect, useRef } from 'react';
import { IconInfoCircle, IconLock, IconMail } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
  /** 'edit' when the host is editing an existing channel: secrets are redacted and optional. */
  mode?: 'create' | 'edit';
};

function encryptionOptions(t: (key: string) => string) {
  return [
    { value: 'none', label: t('encryptionOption.none') },
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: protocol name
    { value: 'starttls', label: 'STARTTLS' },
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: protocol name
    { value: 'ssl_tls', label: 'SSL/TLS' },
  ];
}

function authOptions(t: (key: string) => string) {
  return [
    { value: 'none', label: t('authOption.none') },
    { value: 'plain', label: t('authOption.plain') },
    { value: 'login', label: t('authOption.login') },
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: protocol name
    { value: 'cram_md5', label: 'CRAM-MD5' },
  ];
}

const requiredFields = ['name', 'host', 'port', 'from_address', 'to'];

type FieldSpec = {
  key: string;
  labelKey: string;
  placeholder?: string;
  placeholderKey?: string;
  type?: string;
};

const FIELDS = {
  name: { key: 'name', labelKey: 'nameLabel', placeholderKey: 'namePlaceholder' },
  host: { key: 'host', labelKey: 'hostLabel', placeholder: 'smtp.example.com' },
  port: { key: 'port', labelKey: 'portLabel', placeholder: '587' },
  username: { key: 'username', labelKey: 'usernameLabel', placeholder: 'alerts@example.com' },
  password: { key: 'password', labelKey: 'passwordLabel', placeholder: '********', type: 'password' },
  from_address: { key: 'from_address', labelKey: 'fromAddressLabel', placeholder: 'mcharbor@example.com' },
  from_name: { key: 'from_name', labelKey: 'fromNameLabel', placeholder: 'McHarbor' },
  to: { key: 'to', labelKey: 'toLabel', placeholder: 'ops@example.com, oncall@example.com' },
  subject_prefix: { key: 'subject_prefix', labelKey: 'subjectPrefixLabel', placeholder: '[McHarbor]' },
} satisfies Record<string, FieldSpec>;

export function ConfigForm({ value, onChange, onError, t: hostT, mode }: Props) {
  const isEdit = mode === 'edit';
  const encryption = normalizeEncryption(value.encryption ?? value.security);
  const authMethod = value.auth_method || inferAuthMethod(value);
  const showCredentials = authMethod !== 'none';

  const t = (key: string) => hostT?.(key) ?? key;
  const latest = useRef({ onError, t });
  useEffect(() => {
    latest.current = { onError, t };
  });
  useEffect(() => {
    const { onError: report, t: translate } = latest.current;
    for (const field of requiredFields) {
      report?.(field, fieldValue(value, field).trim() ? null : translate('requiredField'));
    }
    if (showCredentials) {
      report?.('username', value.username?.trim() ? null : translate('usernameRequired'));
      report?.('password', isEdit || value.password?.trim() ? null : translate('passwordRequired'));
    } else {
      report?.('username', null);
      report?.('password', null);
    }
  }, [value, showCredentials, isEdit]);

  function set(key: string, nextValue: string) {
    const next = { ...value, [key]: nextValue };
    if (key === 'auth_method' && nextValue === 'none') {
      delete next.username;
      delete next.password;
    }
    if (key === 'encryption') {
      delete next.security;
    }
    if (key === 'from_address') {
      delete next.from;
    }
    onChange(next);
  }

  function textField(spec: FieldSpec) {
    const { key, type = 'text' } = spec;
    const label = t(spec.labelKey);
    const placeholder = spec.placeholderKey ? t(spec.placeholderKey) : spec.placeholder;
    const id = `comm-smtp-${key}`;
    const effectivePlaceholder = isEdit && key === 'password' ? t('secretKeepHint') : placeholder;
    return (
      <div>
        <Label htmlFor={id} className="mb-1">
          {label}
        </Label>
        <Input
          id={id}
          type={type}
          value={fieldValue(value, key)}
          placeholder={effectivePlaceholder}
          autoComplete="off"
          spellCheck={false}
          onChange={(e: ChangeEvent<HTMLInputElement>) => set(key, e.target.value)}
        />
      </div>
    );
  }

  return (
    <Stack gap={4}>
      <Section icon={<IconInfoCircle className="size-4" />} title={t('title')} help={t('subtitle')}>
        <p className="text-sm text-foreground">{t('description')}</p>
      </Section>

      {textField(FIELDS.name)}

      <Grid columns={2} gap={4}>
        {textField(FIELDS.host)}
        {textField(FIELDS.port)}
      </Grid>

      <Grid columns={2} gap={4}>
        <div>
          <Label htmlFor="comm-smtp-encryption" className="mb-1">
            {t('encryptionLabel')}
          </Label>
          <Select
            value={encryption}
            onChange={(next) => set('encryption', next)}
            options={encryptionOptions(t)}
            searchable={false}
            ariaLabel={t('encryptionLabel')}
          />
        </div>
        <div>
          <Label htmlFor="comm-smtp-auth-method" className="mb-1">
            {t('authMethodLabel')}
          </Label>
          <Select
            value={authMethod}
            onChange={(next) => set('auth_method', next)}
            options={authOptions(t)}
            searchable={false}
            ariaLabel={t('authMethodLabel')}
          />
        </div>
      </Grid>

      {showCredentials && (
        <Grid columns={2} gap={4}>
          {textField(FIELDS.username)}
          {textField(FIELDS.password)}
        </Grid>
      )}

      <Grid columns={2} gap={4}>
        {textField(FIELDS.from_address)}
        {textField(FIELDS.from_name)}
      </Grid>

      <Grid columns={2} gap={4}>
        {textField(FIELDS.to)}
        {textField(FIELDS.subject_prefix)}
      </Grid>

      <Section icon={<IconLock className="size-4" />} title={t('securityTitle')} help={t('securityHelp')}>
        <p className="text-sm text-foreground">{t('securityDescription')}</p>
      </Section>

      <Section icon={<IconMail className="size-4" />} title={t('testTitle')} help={t('testHelp')}>
        <p className="text-sm text-foreground">{t('testDescription')}</p>
      </Section>
    </Stack>
  );
}

function fieldValue(value: Record<string, string>, key: string) {
  if (key === 'from_address') return value.from_address ?? value.from ?? '';
  if (key === 'encryption') return normalizeEncryption(value.encryption ?? value.security);
  if (key === 'auth_method') return value.auth_method || inferAuthMethod(value);
  return value[key] ?? defaultValue(key);
}

function defaultValue(key: string) {
  switch (key) {
    case 'port':
      return '587';
    case 'subject_prefix':
      return '[McHarbor]';
    default:
      return '';
  }
}

function normalizeEncryption(value: string | undefined) {
  switch ((value ?? '').trim().toLowerCase()) {
    case 'tls':
      return 'ssl_tls';
    case 'none':
      return 'none';
    case 'ssl_tls':
      return 'ssl_tls';
    default:
      return 'starttls';
  }
}

function inferAuthMethod(value: Record<string, string>) {
  return value.username || value.password ? 'plain' : 'none';
}
