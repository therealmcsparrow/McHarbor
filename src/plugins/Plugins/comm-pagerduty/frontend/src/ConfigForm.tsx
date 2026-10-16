// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { useEffect } from 'react';
import { Field } from '@resources/components/ui/Form';
import { Input } from '@resources/components/ui/Input';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';

export type Translate = (key: string, vars?: Record<string, unknown>) => string;

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t: Translate;
  mode?: 'create' | 'edit';
};

const SECRET_FIELDS = ['routing_key', 'rest_api_key', 'webhook_signing_secret'] as const;

export function ConfigForm({ value, onChange, onError, t, mode }: Props) {
  const isEdit = mode === 'edit';
  const set = (key: string, next: string) => onChange({ ...value, [key]: next });

  useEffect(() => {
    onError?.('name', (value.name ?? '').trim() ? null : t('form.errors.required'));
    const key = (value.routing_key ?? '').trim();
    const keyOk = (isEdit && key === '') || key.length === 32;
    onError?.('routing_key', keyOk ? null : t('form.errors.routingKeyLength'));
    const poll = value.poll_interval_minutes ?? '5';
    const pollOk = poll === '' || (Number(poll) >= 0 && Number(poll) <= 60);
    onError?.('poll_interval_minutes', pollOk ? null : t('form.errors.pollRange'));
  }, [value, isEdit, onError, t]);

  return (
    <Stack gap={4}>
      <Grid columns={2} gap={4}>
        <Field id="comm-pagerduty-name" label={t('form.name.label')} required>
          <Input
            id="comm-pagerduty-name"
            value={value.name ?? ''}
            placeholder={t('form.name.placeholder')}
            onChange={(e: ChangeEvent<HTMLInputElement>) => set('name', e.target.value)}
          />
        </Field>
        <Field id="comm-pagerduty-region" label={t('form.region.label')} description={t('form.region.help')} required>
          <Select
            value={value.region || 'us'}
            onChange={(next: string) => set('region', next)}
            searchable={false}
            ariaLabel={t('form.region.label')}
            options={[
              { value: 'us', label: t('form.region.us') },
              { value: 'eu', label: t('form.region.eu') },
            ]}
          />
        </Field>
      </Grid>
      {SECRET_FIELDS.map((key) => (
        <Field
          key={key}
          id={`comm-pagerduty-${key}`}
          label={t(`form.${key}.label`)}
          description={t(`form.${key}.help`)}
          required={key === 'routing_key' && !isEdit}
        >
          <Input
            id={`comm-pagerduty-${key}`}
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={value[key] ?? ''}
            placeholder={isEdit ? t('form.secretKeepHint') : t(`form.${key}.placeholder`)}
            onChange={(e: ChangeEvent<HTMLInputElement>) => set(key, e.target.value)}
          />
        </Field>
      ))}
      <Grid columns={2} gap={4}>
        <Field
          id="comm-pagerduty-repeat"
          label={t('form.update_on_repeat.label')}
          description={t('form.update_on_repeat.help')}
        >
          <Switch
            id="comm-pagerduty-repeat"
            checked={value.update_on_repeat !== 'false'}
            onCheckedChange={(on: boolean) => set('update_on_repeat', on ? 'true' : 'false')}
          />
        </Field>
        <Field
          id="comm-pagerduty-poll"
          label={t('form.poll_interval_minutes.label')}
          description={t('form.poll_interval_minutes.help')}
        >
          <NumberInput
            id="comm-pagerduty-poll"
            nullable
            min={0}
            value={
              (value.poll_interval_minutes ?? '5').trim() === '' ? null : Number(value.poll_interval_minutes ?? '5')
            }
            onChange={(next) => set('poll_interval_minutes', next === null ? '' : String(Math.trunc(next)))}
          />
        </Field>
      </Grid>
      <Field
        id="comm-pagerduty-severity_map"
        label={t('form.severity_map.label')}
        description={t('form.severity_map.help')}
      >
        <Input
          id="comm-pagerduty-severity_map"
          value={value.severity_map ?? ''}
          placeholder={t('form.severity_map.placeholder')}
          spellCheck={false}
          onChange={(e: ChangeEvent<HTMLInputElement>) => set('severity_map', e.target.value)}
        />
      </Field>
    </Stack>
  );
}
