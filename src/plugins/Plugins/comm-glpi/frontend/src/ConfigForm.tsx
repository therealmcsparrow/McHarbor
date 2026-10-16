// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Field } from '@resources/components/ui/Form';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Select } from '@resources/components/ui/Select';
import { Subheading, Text as TextBlock } from '@resources/components/ui/Typography';
import { TextField, ToggleField, type FieldProps, type Translate } from './fields';
import { useGlpiValidation } from './useGlpiValidation';
import { V1Fields } from './V1Fields';
import { V2Fields } from './V2Fields';

export type { Translate };

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t: Translate;
  mode?: 'create' | 'edit';
};

const INSTANCE_URL_EXAMPLE = 'https://glpi.example.org';
const SEVERITY_MAP_EXAMPLE = '{"critical":{"urgency":5,"impact":5,"priority":6}}';

export function ConfigForm({ value, onChange, onError, t, mode }: Props) {
  const isEdit = mode === 'edit';
  const errors = useGlpiValidation(value, isEdit, t, onError);
  const set = (key: string, next: string) => onChange({ ...value, [key]: next });
  const ctx: FieldProps = { value, set, errors, t, isEdit };
  const api = value.api_version || 'v1';

  return (
    <Stack gap={4}>
      <Stack gap={1}>
        <Subheading size="base" weight="semibold">
          {t('form.title')}
        </Subheading>
        <TextBlock tone="muted">{t('form.intro')}</TextBlock>
      </Stack>
      <Grid columns={2} gap={4}>
        <TextField name="name" label={t('form.name')} required ctx={ctx} />
        <TextField
          name="instance_url"
          label={t('form.instanceUrl')}
          help={t('form.instanceUrlHelp')}
          placeholder={INSTANCE_URL_EXAMPLE}
          required
          ctx={ctx}
        />
        <Field id="comm-glpi-api_version" label={t('form.apiVersion')} required>
          <Select
            value={api}
            onChange={(next: string) => set('api_version', next)}
            searchable={false}
            ariaLabel={t('form.apiVersion')}
            options={[
              { value: 'v1', label: t('form.apiV1') },
              { value: 'v2', label: t('form.apiV2') },
            ]}
          />
        </Field>
      </Grid>
      {api === 'v2' ? <V2Fields ctx={ctx} /> : <V1Fields ctx={ctx} />}
      <Grid columns={2} gap={4}>
        <TextField
          name="entities_id"
          kind="id"
          label={t('form.entityId')}
          help={t('form.entityHelp')}
          fallback="0"
          ctx={ctx}
        />
        <TextField name="itilcategories_id" kind="id" label={t('form.categoryId')} fallback="0" ctx={ctx} />
        <TextField
          name="default_assignment_group"
          kind="id"
          label={t('form.groupId')}
          help={t('form.groupHelp')}
          fallback="0"
          ctx={ctx}
        />
        <TextField
          name="poll_interval_minutes"
          kind="int"
          label={t('form.pollInterval')}
          help={t('form.pollHelp')}
          fallback="5"
          ctx={ctx}
        />
        <TextField
          name="reopen_window_minutes"
          kind="int"
          label={t('form.reopenWindow')}
          help={t('form.reopenHelp')}
          fallback="0"
          ctx={ctx}
        />
        <ToggleField name="close_on_resolve" label={t('form.closeOnResolve')} fallback={false} ctx={ctx} />
        <ToggleField name="update_on_repeat" label={t('form.updateOnRepeat')} fallback ctx={ctx} />
      </Grid>
      <TextField
        name="webhook_signing_secret"
        kind="secret"
        label={t('form.webhookSecret')}
        help={t('form.webhookHelp')}
        ctx={ctx}
      />
      <TextField
        name="severity_map"
        label={t('form.severityMap')}
        help={t('form.severityMapHelp')}
        placeholder={SEVERITY_MAP_EXAMPLE}
        ctx={ctx}
      />
    </Stack>
  );
}
