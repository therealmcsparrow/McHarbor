// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { Alert } from '@resources/components/ui/Alert';
import { Field } from '@resources/components/ui/Form';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Select } from '@resources/components/ui/Select';
import { SwitchField, TextField } from './fields';
import { ProfileHint } from './ProfileHint';
import { AUTH_MODES, PROFILES, SECRET_KEYS, authModeOf, profileOf, urlError, type Translate } from './options';

export type { Translate } from './options';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t: Translate;
};

export function ConfigForm({ value, onChange, onError, t }: Props) {
  const profile = profileOf(value);
  const authMode = authModeOf(value);
  const urlErrorKey = urlError(value);
  const urlErrorText = urlErrorKey ? t(urlErrorKey) : null;

  const errorReporter = useRef(onError);
  useEffect(() => {
    errorReporter.current = onError;
  });
  useEffect(() => {
    errorReporter.current?.('webhook_url', urlErrorText);
  }, [urlErrorText]);

  const set = (key: string, v: string) => {
    const next = { ...value, [key]: v };
    if (v === '' && (SECRET_KEYS as readonly string[]).includes(key)) delete next[key];
    onChange(next);
  };
  const text = (key: string) => value[key] ?? '';
  const flag = (key: string, fallback: boolean) => (value[key] === undefined ? fallback : value[key] === 'true');

  const profileOptions = PROFILES.map((p) => ({ value: p, label: t(`profiles.${p}.label`) }));
  const authOptions = AUTH_MODES.map((m) => ({ value: m, label: t(`form.auth_mode.${m}`) }));

  return (
    <Stack gap={4}>
      <Grid columns={2} gap={4}>
        <TextField name="name" value={text('name')} required t={t} onChange={set} />
        <Field id="alertmanager-webhook-profile" label={t('form.profile.label')}>
          <Select
            value={profile}
            options={profileOptions}
            searchable={false}
            ariaLabel={t('form.profile.label')}
            onChange={(v) => set('profile', v)}
          />
        </Field>
      </Grid>
      <ProfileHint profile={profile} t={t} />
      <TextField
        name="webhook_url"
        value={text('webhook_url')}
        secret
        required
        help
        error={urlErrorText}
        t={t}
        onChange={set}
      />
      <Grid columns={2} gap={4}>
        <TextField name="receiver_name" value={text('receiver_name')} help t={t} onChange={set} />
        <Field id="alertmanager-webhook-auth_mode" label={t('form.auth_mode.label')}>
          <Select
            value={authMode}
            options={authOptions}
            searchable={false}
            ariaLabel={t('form.auth_mode.label')}
            onChange={(v) => set('auth_mode', v)}
          />
        </Field>
        {authMode === 'bearer' && (
          <TextField name="bearer_token" value={text('bearer_token')} secret required t={t} onChange={set} />
        )}
        {authMode === 'basic' && (
          <>
            <TextField name="basic_username" value={text('basic_username')} t={t} onChange={set} />
            <TextField name="basic_password" value={text('basic_password')} secret t={t} onChange={set} />
          </>
        )}
        <TextField name="hmac_secret" value={text('hmac_secret')} secret help t={t} onChange={set} />
        <TextField name="severity_map" value={text('severity_map')} help t={t} onChange={set} />
      </Grid>
      <Grid columns={3} gap={4}>
        <SwitchField name="allow_http" checked={flag('allow_http', false)} t={t} onChange={set} />
        <SwitchField name="update_on_repeat" checked={flag('update_on_repeat', true)} help t={t} onChange={set} />
        <SwitchField name="test_sends_alert" checked={flag('test_sends_alert', false)} help t={t} onChange={set} />
      </Grid>
      {value.allow_http === 'true' && <Alert variant="warning">{t('form.insecureWarning')}</Alert>}
    </Stack>
  );
}
