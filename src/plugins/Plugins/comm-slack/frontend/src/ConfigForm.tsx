// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect, useRef } from 'react';
import { IconBrandSlack, IconInfoCircle, IconLock } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Stack } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
  /** 'edit' when the host is editing an existing channel: secrets are redacted and optional. */
  mode?: 'create' | 'edit';
};

const requiredFields = ['name'];
const secretFields = ['webhook_url'];

type FieldSpec = {
  key: string;
  labelKey: string;
  placeholder?: string;
  placeholderKey?: string;
  type?: string;
};

const FIELDS = {
  name: { key: 'name', labelKey: 'nameLabel', placeholderKey: 'namePlaceholder' },
  webhook_url: {
    key: 'webhook_url',
    labelKey: 'webhookUrlLabel',
    placeholder: 'https://hooks.slack.com/services/...',
    type: 'password',
  },
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
    if (key === 'webhook_url') {
      delete next.webhookUrl;
    }
    onChange(next);
  }

  function textField(spec: FieldSpec) {
    const { key, type = 'text' } = spec;
    const label = t(spec.labelKey);
    const placeholder = spec.placeholderKey ? t(spec.placeholderKey) : spec.placeholder;
    const id = `comm-slack-${key}`;
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
      {textField(FIELDS.webhook_url)}

      <Section icon={<IconLock className="size-4" />} title={t('securityTitle')} help={t('securityHelp')}>
        <p className="text-sm text-foreground">{t('securityDescription')}</p>
      </Section>

      <Section icon={<IconBrandSlack className="size-4" />} title={t('testTitle')} help={t('testHelp')}>
        <p className="text-sm text-foreground">{t('testDescription')}</p>
      </Section>
    </Stack>
  );
}

function fieldValue(value: Record<string, string>, key: string) {
  if (key === 'webhook_url') return value.webhook_url ?? value.webhookUrl ?? '';
  return value[key] ?? '';
}
