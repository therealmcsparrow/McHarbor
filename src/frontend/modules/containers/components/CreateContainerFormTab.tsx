// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Ref } from 'react';
import { useTranslation } from 'react-i18next';
import type { NetworkInfo } from '@core/types/docker';
import { Input } from '@resources/components/ui/Input';
import { Box, Flex } from '@resources/components/ui/Layout';
import { FieldLabel, FieldHelp } from '@resources/components/ui/Form';
import { NetworkSection } from './CreateContainerNetworkSection';
import { CreateContainerGpuSection } from './CreateContainerGpuSection';
import { CreateContainerDevicesSection } from './CreateContainerDevicesSection';
import { CreateContainerPortsSection } from './CreateContainerPortsSection';
import { CreateContainerEnvSection } from './CreateContainerEnvSection';
import { CreateContainerVolumesSection } from './CreateContainerVolumesSection';
import { CreateContainerOptionsSection } from './CreateContainerOptionsSection';
import { CreateContainerAdvancedSection } from './CreateContainerAdvancedSection';
import type { CreateContainerForm } from './useCreateContainerForm';

type CreateContainerFormTabProps = {
  form: CreateContainerForm;
  existingNetworks: NetworkInfo[];
  onNetworkCreated: (network: NetworkInfo) => void;
  imageInputRef?: Ref<HTMLInputElement>;
};

export function CreateContainerFormTab({
  form,
  existingNetworks,
  onNetworkCreated,
  imageInputRef,
}: CreateContainerFormTabProps) {
  const { t } = useTranslation('containers');
  return (
    <Box className="space-y-4">
      <Flex gap={3}>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.image')}</FieldLabel>
          <Input
            ref={imageInputRef}
            value={form.image}
            onChange={(e) => form.setImage(e.target.value)}
            placeholder={t('create.form.imagePlaceholder')}
            className="font-mono"
          />
          <FieldHelp>{t('create.form.imageHelp')}</FieldHelp>
        </Box>
        <Box className="flex-1">
          <FieldLabel>{t('create.form.name')}</FieldLabel>
          <Input
            value={form.entryName}
            onChange={(e) => form.setName(e.target.value)}
            placeholder={t('create.form.namePlaceholder')}
          />
        </Box>
      </Flex>

      <CreateContainerPortsSection ports={form.ports} setPorts={form.setPorts} />
      <CreateContainerEnvSection
        env={form.env}
        setEnv={form.setEnv}
        secretFileMode={form.secretFileMode}
        setSecretFileMode={form.setSecretFileMode}
      />
      <CreateContainerVolumesSection volumes={form.volumes} setVolumes={form.setVolumes} />
      <NetworkSection
        network={form.network}
        setNetwork={form.setNetwork}
        existingNetworks={existingNetworks}
        onNetworkCreated={onNetworkCreated}
      />
      <CreateContainerGpuSection value={form.gpu} onChange={form.setGpu} />
      <CreateContainerDevicesSection value={form.devices} onChange={form.setDevices} />
      <CreateContainerOptionsSection form={form} />
      <CreateContainerAdvancedSection form={form} />
    </Box>
  );
}
