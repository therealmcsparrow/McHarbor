// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect, useRef } from 'react';
import { IconInfoCircle, IconKey, IconMail } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
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

const requiredFields = ['name', 'tenant_id', 'client_id', 'from_address', 'to'];
const secretFields = ['client_secret'];

type FieldSpec = {
  key: string;
  labelKey: string;
  placeholder?: string;
  placeholderKey?: string;
  type?: string;
};

const FIELDS = {
  name: { key: 'name', labelKey: 'nameLabel', placeholderKey: 'namePlaceholder' },
  tenant_id: { key: 'tenant_id', labelKey: 'tenantIdLabel', placeholder: '00000000-0000-0000-0000-000000000000' },
  client_id: { key: 'client_id', labelKey: 'clientIdLabel', placeholder: '00000000-0000-0000-0000-000000000000' },
  client_secret: { key: 'client_secret', labelKey: 'clientSecretLabel', placeholder: '********', type: 'password' },
  from_address: { key: 'from_address', labelKey: 'fromAddressLabel', placeholder: 'mcharbor@example.com' },
  from_name: { key: 'from_name', labelKey: 'fromNameLabel', placeholder: 'McHarbor' },
  to: { key: 'to', labelKey: 'toLabel', placeholder: 'ops@example.com, oncall@example.com' },
  subject_prefix: { key: 'subject_prefix', labelKey: 'subjectPrefixLabel', placeholder: '[McHarbor]' },
} satisfies Record<string, FieldSpec>;

export function ConfigForm({ value, onChange, onError, t: hostT, mode }: Props) {
  const isEdit = mode === 'edit';
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
    for (const field of secretFields) {
      report?.(field, isEdit || fieldValue(value, field).trim() ? null : translate('requiredField'));
    }
  }, [value, isEdit]);

  function set(key: string, nextValue: string) {
    const next = { ...value, [key]: nextValue };
    if (key === 'from_address') {
      delete next.from;
    }
    onChange(next);
  }

  function textField(spec: FieldSpec) {
    const { key, type = 'text' } = spec;
    const label = t(spec.labelKey);
    const placeholder = spec.placeholderKey ? t(spec.placeholderKey) : spec.placeholder;
    const id = `comm-exchange-${key}`;
    const effectivePlaceholder = isEdit && secretFields.includes(key) ? t('secretKeepHint') : placeholder;
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

      {textField(FIELDS.tenant_id)}

      <Grid columns={2} gap={4}>
        {textField(FIELDS.client_id)}
        {textField(FIELDS.client_secret)}
      </Grid>

      <Grid columns={2} gap={4}>
        {textField(FIELDS.from_address)}
        {textField(FIELDS.from_name)}
      </Grid>

      <Grid columns={2} gap={4}>
        {textField(FIELDS.to)}
        {textField(FIELDS.subject_prefix)}
      </Grid>

      <Section icon={<IconKey className="size-4" />} title={t('securityTitle')} help={t('securityHelp')}>
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
  return value[key] ?? defaultValue(key);
}

function defaultValue(key: string) {
  switch (key) {
    case 'subject_prefix':
      return '[McHarbor]';
    default:
      return '';
  }
}
