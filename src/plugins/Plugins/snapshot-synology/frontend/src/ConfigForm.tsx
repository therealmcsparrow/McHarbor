// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ConfigForm for the snapshot-synology plugin. Renders the DSM
// connection fields (URL, username, password, share, certificate
// toggle) and bubbles typed values up to the host dialog via
// `onChange`. The host owns submit, validation, and audit.

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

const DSM_URL_EXAMPLE = 'https://nas.local:5001';
const SHARE_EXAMPLE = 'backups';

export function ConfigForm({ formData, onChange }: ConfigFormProps) {
  const { t } = useTranslation('snapshot-synology');

  const set = (key: string, value: unknown) => {
    onChange({ ...formData, [key]: value });
  };

  return (
    <Stack>
      <Section title={t('form.connectionSection')}>
        <Field label={t('form.dsmUrl.label')} description={t('form.dsmUrl.help')}>
          <Input
            value={(formData.dsm_url as string) ?? ''}
            onChange={(e) => set('dsm_url', e.target.value)}
            placeholder={DSM_URL_EXAMPLE}
            type="url"
          />
        </Field>
        <Field label={t('form.username.label')} description={t('form.username.help')}>
          <Input
            value={(formData.username as string) ?? ''}
            onChange={(e) => set('username', e.target.value)}
            autoComplete="username"
          />
        </Field>
        <Field label={t('form.password.label')} description={t('form.password.help')}>
          <Input
            value={(formData.password as string) ?? ''}
            onChange={(e) => set('password', e.target.value)}
            type="password"
            autoComplete="current-password"
          />
        </Field>
      </Section>

      <Section title={t('form.targetSection')}>
        <Field label={t('form.shareName.label')} description={t('form.shareName.help')}>
          <Input
            value={(formData.share_name as string) ?? ''}
            onChange={(e) => set('share_name', e.target.value)}
            placeholder={SHARE_EXAMPLE}
          />
        </Field>
      </Section>

      <Section title={t('form.advancedSection')}>
        <Field label={t('form.ignoreCert.label')} description={t('form.ignoreCert.help')}>
          <Switch
            checked={formData.ignore_certificate_errors === true}
            onCheckedChange={(v) => set('ignore_certificate_errors', v)}
          />
        </Field>
      </Section>

      <Alert variant="info">{t('form.appPasswordHint')}</Alert>
    </Stack>
  );
}
