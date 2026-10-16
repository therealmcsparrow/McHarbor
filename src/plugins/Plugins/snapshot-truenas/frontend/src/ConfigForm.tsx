// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm for the snapshot-truenas plugin. Renders the
// middleware API connection fields (API URL, API key, dataset path,
// TLS verification toggle) and bubbles typed values up to the host
// dialog via `onChange`. The host owns submit, validation, and
// audit.

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { Stack } from '@resources/components/ui/Layout';
import { Section, Field } from '@resources/components/ui/Form';
import { Alert } from '@resources/components/ui/Alert';

export interface ConfigFormProps {
  driver: string;
  formData: Record<string, unknown>;
  onChange: (data: Record<string, unknown>) => void;
}

const API_URL_EXAMPLE = 'https://nas.local/api/v2.0';
const DATASET_EXAMPLE = 'tank/backups';

export function ConfigForm({ formData, onChange }: ConfigFormProps) {
  const { t } = useTranslation('snapshot-truenas');

  const set = (key: string, value: unknown) => {
    onChange({ ...formData, [key]: value });
  };

  // Default verify_tls to true when unset so the off-state is
  // explicit (matches the form field help text "Disable to
  // accept self-signed certificates").
  const verifyTls = formData.verify_tls !== false;

  return (
    <Stack>
      <Section title={t('form.connectionSection')}>
        <Field label={t('form.apiUrl.label')} description={t('form.apiUrl.help')}>
          <Input
            value={(formData.api_url as string) ?? ''}
            onChange={(e) => set('api_url', e.target.value)}
            placeholder={API_URL_EXAMPLE}
            type="url"
          />
        </Field>
        <Field label={t('form.apiKey.label')} description={t('form.apiKey.help')}>
          <Input
            value={(formData.api_key as string) ?? ''}
            onChange={(e) => set('api_key', e.target.value)}
            type="password"
            autoComplete="off"
          />
        </Field>
      </Section>

      <Section title={t('form.targetSection')}>
        <Field label={t('form.datasetPath.label')} description={t('form.datasetPath.help')}>
          <Input
            value={(formData.dataset_path as string) ?? ''}
            onChange={(e) => set('dataset_path', e.target.value)}
            placeholder={DATASET_EXAMPLE}
          />
        </Field>
      </Section>

      <Section title={t('form.advancedSection')}>
        <Field label={t('form.verifyTls.label')} description={t('form.verifyTls.help')}>
          <Switch checked={verifyTls} onCheckedChange={(v) => set('verify_tls', v)} />
        </Field>
      </Section>

      <Alert variant="info">{t('form.recursiveHint')}</Alert>
    </Stack>
  );
}
