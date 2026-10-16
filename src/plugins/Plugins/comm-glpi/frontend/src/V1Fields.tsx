// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Grid } from '@resources/components/ui/Layout';
import { TextField, type FieldProps } from './fields';

export function V1Fields({ ctx }: { ctx: FieldProps }) {
  const { t } = ctx;
  return (
    <Grid columns={2} gap={4}>
      <TextField name="app_token" kind="secret" label={t('form.appToken')} help={t('form.appTokenHelp')} ctx={ctx} />
      <TextField
        name="user_token"
        kind="secret"
        label={t('form.userToken')}
        help={t('form.userTokenHelp')}
        required={!ctx.isEdit}
        ctx={ctx}
      />
    </Grid>
  );
}
