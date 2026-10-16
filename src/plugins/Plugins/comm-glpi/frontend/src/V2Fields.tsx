// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Grid } from '@resources/components/ui/Layout';
import { TextField, type FieldProps } from './fields';

export function V2Fields({ ctx }: { ctx: FieldProps }) {
  const { t } = ctx;
  return (
    <Grid columns={2} gap={4}>
      <TextField name="v2_path" label={t('form.v2Path')} help={t('form.v2PathHelp')} fallback="v2.0" ctx={ctx} />
      <TextField name="oauth_client_id" label={t('form.oauthClientId')} required ctx={ctx} />
      <TextField
        name="oauth_client_secret"
        kind="secret"
        label={t('form.oauthClientSecret')}
        required={!ctx.isEdit}
        ctx={ctx}
      />
      <TextField name="oauth_username" label={t('form.oauthUsername')} required ctx={ctx} />
      <TextField name="oauth_password" kind="secret" label={t('form.oauthPassword')} required={!ctx.isEdit} ctx={ctx} />
    </Grid>
  );
}
