// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect, useRef } from 'react';
import { IconBell, IconInfoCircle, IconLock } from '@tabler/icons-react';
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

const requiredFields = ['name', 'server_url'];
const secretFields = ['token'];

type FieldSpec = {
  key: string;
  labelKey: string;
  placeholder?: string;
  placeholderKey?: string;
  type?: string;
};

const FIELDS = {
  name: { key: 'name', labelKey: 'nameLabel', placeholderKey: 'namePlaceholder' },
  server_url: { key: 'server_url', labelKey: 'serverUrlLabel', placeholder: 'https://gotify.example.com' },
  token: { key: 'token', labelKey: 'tokenLabel', placeholder: '********', type: 'password' },
  priority: { key: 'priority', labelKey: 'priorityLabel', placeholder: '5' },
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
    if (key === 'server_url') {
      delete next.serverUrl;
    }
    onChange(next);
  }

  function textField(spec: FieldSpec) {
    const { key, type = 'text' } = spec;
    const label = t(spec.labelKey);
    const placeholder = spec.placeholderKey ? t(spec.placeholderKey) : spec.placeholder;
    const id = `comm-gotify-${key}`;
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

      <Grid columns={2} gap={4}>
        {textField(FIELDS.server_url)}
        {textField(FIELDS.token)}
      </Grid>

      {textField(FIELDS.priority)}

      <Section icon={<IconLock className="size-4" />} title={t('securityTitle')} help={t('securityHelp')}>
        <p className="text-sm text-foreground">{t('securityDescription')}</p>
      </Section>

      <Section icon={<IconBell className="size-4" />} title={t('testTitle')} help={t('testHelp')}>
        <p className="text-sm text-foreground">{t('testDescription')}</p>
      </Section>
    </Stack>
  );
}

function fieldValue(value: Record<string, string>, key: string) {
  if (key === 'server_url') return value.server_url ?? value.serverUrl ?? '';
  return value[key] ?? '';
}
