// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Grid, Stack } from '@resources/components/ui/Layout';
import { Heading, Text as TextBlock } from '@resources/components/ui/Typography';
import { SwitchField, TextField, type SetField, type Translate } from './fields';
import type { ItopErrors } from './useItopValidation';

type Props = {
  text: (key: string, fallback?: string) => string;
  flag: (key: string, fallback: boolean) => boolean;
  errors: ItopErrors;
  set: SetField;
  t: Translate;
};

export function LifecycleFields({ text, flag, errors, set, t }: Props) {
  return (
    <Stack gap={3}>
      <Heading as="h3" size="base" weight="semibold">
        {t('form.lifecycleTitle')}
      </Heading>
      <TextBlock tone="muted">{t('form.lifecycleHelp')}</TextBlock>
      <Grid columns={2} gap={4}>
        <TextField
          name="team_id"
          label={t('form.teamId')}
          kind="id"
          required
          value={text('team_id')}
          error={errors.team_id}
          onChange={set}
        />
        <TextField
          name="agent_id"
          label={t('form.agentId')}
          kind="id"
          required
          value={text('agent_id')}
          error={errors.agent_id}
          onChange={set}
        />
        <TextField
          name="service_id"
          label={t('form.serviceId')}
          kind="id"
          required
          value={text('service_id')}
          error={errors.service_id}
          onChange={set}
        />
        <TextField
          name="resolution_code"
          label={t('form.resolutionCode')}
          value={text('resolution_code', 'other')}
          onChange={set}
        />
        <SwitchField
          name="close_on_resolve"
          label={t('form.closeOnResolve')}
          checked={flag('close_on_resolve', false)}
          onChange={set}
        />
      </Grid>
    </Stack>
  );
}
