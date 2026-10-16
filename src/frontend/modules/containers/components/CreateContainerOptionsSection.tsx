// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { FieldLabel } from '@resources/components/ui/Form';
import type { RestartPolicy } from '../hooks/useCreateContainer';
import { RESTART_OPTIONS } from './createContainerFormHelpers';
import type { CreateContainerForm } from './useCreateContainerForm';

function ToggleRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <Flex align="center" justify="between" gap={2}>
      <BodyText size="sm">{label}</BodyText>
      <Switch checked={checked} onCheckedChange={onChange} />
    </Flex>
  );
}

export function CreateContainerOptionsSection({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('containers');
  return (
    <Box className="grid grid-cols-2 gap-x-6 gap-y-3 rounded-md border border-border bg-muted/20 p-3">
      <Box>
        <FieldLabel>{t('create.form.restart')}</FieldLabel>
        <Select
          value={form.restart}
          onChange={(v) => form.setRestart(v as RestartPolicy)}
          options={RESTART_OPTIONS}
          searchable={false}
        />
      </Box>
      <Box />
      <ToggleRow label={t('create.form.tty')} checked={form.tty} onChange={form.setTty} />
      <ToggleRow label={t('create.form.stdin')} checked={form.stdin} onChange={form.setStdin} />
      <ToggleRow label={t('create.form.autoRemove')} checked={form.autoRemove} onChange={form.setAutoRemove} />
      <ToggleRow label={t('create.form.autoStart')} checked={form.autoStart} onChange={form.setAutoStart} />
      <ToggleRow label={t('create.form.pullImage')} checked={form.pullImage} onChange={form.setPullImage} />
    </Box>
  );
}
