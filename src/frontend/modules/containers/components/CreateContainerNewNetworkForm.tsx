// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select, type SelectOption } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as BodyText } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { FieldLabel, FieldHelp } from '@resources/components/ui/Form';
import { DRIVER_CONFIG, type Driver } from '@resources/constants/network-drivers';
import type { NewNetworkDraft } from './useNewNetworkDraft';

type CreateContainerNewNetworkFormProps = {
  draft: NewNetworkDraft;
  driverOptions: SelectOption[];
};

export function CreateContainerNewNetworkForm({ draft, driverOptions }: CreateContainerNewNetworkFormProps) {
  const { t } = useTranslation('containers');
  const { createNetwork, newDriver } = draft;
  return (
    <Box className="space-y-3 rounded-md border border-dashed border-border bg-background/40 p-3">
      <FieldHelp>{t('create.form.networkNewHelp')}</FieldHelp>
      <Flex gap={3}>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.networkNewName')}</FieldLabel>
          <Input
            value={draft.newName}
            onChange={(e) => draft.setNewName(e.target.value)}
            placeholder={t('create.form.networkNewNamePlaceholder')}
            className="font-mono"
          />
        </Box>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.networkNewDriver')}</FieldLabel>
          <Select
            value={newDriver}
            onChange={(v) => draft.setNewDriver(v as Driver)}
            options={driverOptions}
            searchable={false}
          />
        </Box>
      </Flex>
      {DRIVER_CONFIG[newDriver].hasIpam && (
        <Flex gap={3}>
          <Box className="flex-1">
            <FieldLabel>{t('create.form.networkNewSubnet')}</FieldLabel>
            <Input
              value={draft.newSubnet}
              onChange={(e) => draft.setNewSubnet(e.target.value)}
              placeholder={t('create.form.networkNewSubnetPlaceholder')}
              className="font-mono"
            />
          </Box>
          <Box className="flex-1">
            <FieldLabel>{t('create.form.networkNewGateway')}</FieldLabel>
            <Input
              value={draft.newGateway}
              onChange={(e) => draft.setNewGateway(e.target.value)}
              placeholder={t('create.form.networkNewGatewayPlaceholder')}
              className="font-mono"
            />
          </Box>
        </Flex>
      )}
      {DRIVER_CONFIG[newDriver].hasToggles && (
        <Flex gap={3}>
          <Flex align="center" justify="between" gap={2} className="flex-1">
            <BodyText size="sm">{t('create.form.networkNewInternal')}</BodyText>
            <Switch checked={draft.newInternal} onCheckedChange={draft.setNewInternal} />
          </Flex>
          <Flex align="center" justify="between" gap={2} className="flex-1">
            <BodyText size="sm">{t('create.form.networkNewAttachable')}</BodyText>
            <Switch checked={draft.newAttachable} onCheckedChange={draft.setNewAttachable} />
          </Flex>
        </Flex>
      )}
      <Flex justify="end" gap={2}>
        <Button size="sm" onClick={draft.submit} disabled={!draft.newName.trim() || createNetwork.isPending}>
          {createNetwork.isPending ? <Spinner size="sm" className="mr-2" /> : <IconPlus className="mr-1 size-3.5" />}
          {createNetwork.isPending ? t('create.creating', { ns: 'networks' }) : t('create.form.networkSubmit')}
        </Button>
      </Flex>
      {createNetwork.isError && (
        <Box className="rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
          {createNetwork.error instanceof Error
            ? createNetwork.error.message
            : t('toast.updateFailed', { ns: 'networks' })}
        </Box>
      )}
    </Box>
  );
}
