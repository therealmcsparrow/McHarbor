// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Grid, Stack } from '@resources/components/ui/Layout';
import { Heading, Text as TextBlock } from '@resources/components/ui/Typography';
import { ChoiceField, SECRET_KEYS, SwitchField, TextField, type Translate } from './fields';
import { LifecycleFields } from './LifecycleFields';
import { useItopValidation } from './useItopValidation';

export type { Translate } from './fields';

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t: Translate;
};

export function ConfigForm({ value, onChange, onError, t }: Props) {
  const errors = useItopValidation(value, onError, t);
  const set = (key: string, v: string) => {
    const next = { ...value, [key]: v };
    if (v === '' && (SECRET_KEYS as readonly string[]).includes(key)) delete next[key];
    onChange(next);
  };
  const text = (key: string, fallback = '') => value[key] ?? fallback;
  const flag = (key: string, fallback: boolean) => (value[key] === undefined ? fallback : value[key] === 'true');
  const authMode = text('auth_mode', 'token') === 'password' ? 'password' : 'token';
  const secretHint = t('form.secretKeep');

  return (
    <Stack gap={5}>
      <Stack gap={1}>
        <Heading as="h3" size="base" weight="semibold">
          {t('form.title')}
        </Heading>
        <TextBlock tone="muted">{t('form.intro')}</TextBlock>
      </Stack>
      <Grid columns={2} gap={4}>
        <TextField
          name="name"
          label={t('form.name')}
          required
          value={text('name')}
          error={errors.name}
          onChange={set}
        />
        <TextField
          name="instance_url"
          label={t('form.instanceUrl')}
          required
          placeholder={t('form.instanceUrlPlaceholder')}
          value={text('instance_url')}
          error={errors.instance_url}
          onChange={set}
        />
        <ChoiceField
          name="auth_mode"
          label={t('form.authMode')}
          value={authMode}
          options={[
            { value: 'token', label: t('form.authToken') },
            { value: 'password', label: t('form.authPassword') },
          ]}
          onChange={set}
        />
        {authMode === 'token' ? (
          <TextField
            name="auth_token"
            label={t('form.token')}
            kind="secret"
            description={secretHint}
            value={text('auth_token')}
            onChange={set}
          />
        ) : (
          <>
            <TextField name="auth_user" label={t('form.user')} value={text('auth_user')} onChange={set} />
            <TextField
              name="auth_pwd"
              label={t('form.password')}
              kind="secret"
              description={secretHint}
              value={text('auth_pwd')}
              onChange={set}
            />
          </>
        )}
        <ChoiceField
          name="ticket_class"
          label={t('form.ticketClass')}
          value={text('ticket_class', 'Incident') === 'UserRequest' ? 'UserRequest' : 'Incident'}
          options={[
            { value: 'Incident', label: t('form.classIncident') },
            { value: 'UserRequest', label: t('form.classUserRequest') },
          ]}
          onChange={set}
        />
        <TextField
          name="org_name"
          label={t('form.orgName')}
          required
          value={text('org_name')}
          error={errors.org_name}
          onChange={set}
        />
        <TextField name="caller_email" label={t('form.callerEmail')} value={text('caller_email')} onChange={set} />
        <TextField name="origin" label={t('form.origin')} value={text('origin', 'monitoring')} onChange={set} />
      </Grid>
      <LifecycleFields text={text} flag={flag} errors={errors} set={set} t={t} />
      <Grid columns={2} gap={4}>
        <TextField
          name="default_assignment_group"
          label={t('form.createTeamId')}
          kind="id"
          value={text('default_assignment_group', '0')}
          error={errors.default_assignment_group}
          onChange={set}
        />
        <TextField
          name="reopen_window_minutes"
          label={t('form.reopenWindow')}
          kind="number"
          value={text('reopen_window_minutes', '0')}
          error={errors.reopen_window_minutes}
          onChange={set}
        />
        <TextField
          name="poll_interval_minutes"
          label={t('form.pollInterval')}
          kind="number"
          value={text('poll_interval_minutes', '5')}
          error={errors.poll_interval_minutes}
          onChange={set}
        />
        <SwitchField
          name="update_on_repeat"
          label={t('form.updateOnRepeat')}
          checked={flag('update_on_repeat', true)}
          onChange={set}
        />
      </Grid>
      <TextField
        name="severity_map"
        label={t('form.severityMap')}
        placeholder={t('form.severityMapPlaceholder')}
        value={text('severity_map')}
        error={errors.severity_map}
        onChange={set}
      />
    </Stack>
  );
}
