// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconSettings } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Box, Flex } from '@resources/components/ui/Layout';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { FieldLabel, FieldHelp } from '@resources/components/ui/Form';
import { ENTRYPOINT_EXAMPLE, WORKING_DIR_EXAMPLE } from './createContainerFormHelpers';
import type { CreateContainerForm } from './useCreateContainerForm';

export function CreateContainerAdvancedSection({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('containers');
  return (
    <Disclosure
      title={t('create.form.advanced')}
      description={t('create.form.advancedHelp')}
      icon={<IconSettings className="size-4" />}
      open={form.advancedOpen}
      onOpenChange={form.setAdvancedOpen}
    >
      <Box>
        <FieldLabel>{t('create.form.command')}</FieldLabel>
        <Input
          value={form.cmd}
          onChange={(e) => form.setCmd(e.target.value)}
          placeholder={t('create.form.commandPlaceholder')}
          className="font-mono"
        />
        <FieldHelp>{t('create.form.commandHelp')}</FieldHelp>
      </Box>
      <Box>
        <FieldLabel>{t('create.form.entrypoint')}</FieldLabel>
        <Input
          value={form.entrypoint}
          onChange={(e) => form.setEntrypoint(e.target.value)}
          placeholder={ENTRYPOINT_EXAMPLE}
          className="font-mono"
        />
        <FieldHelp>{t('create.form.entrypointHelp')}</FieldHelp>
      </Box>
      <Flex gap={3}>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.workingDir')}</FieldLabel>
          <Input
            value={form.workingDir}
            onChange={(e) => form.setWorkingDir(e.target.value)}
            placeholder={WORKING_DIR_EXAMPLE}
            className="font-mono"
          />
        </Box>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.user')}</FieldLabel>
          <Input
            value={form.user}
            onChange={(e) => form.setUser(e.target.value)}
            placeholder="1000:1000"
            className="font-mono"
          />
        </Box>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.hostname')}</FieldLabel>
          <Input
            value={form.hostname}
            onChange={(e) => form.setHostname(e.target.value)}
            placeholder={t('create.hostnamePlaceholder')}
            className="font-mono"
          />
        </Box>
      </Flex>
    </Disclosure>
  );
}
