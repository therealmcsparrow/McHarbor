// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { IconContainer, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Caption } from '@resources/components/ui/Typography';
import { Flex } from '@resources/components/ui/Layout';
import { Section } from '@resources/components/ui/Form';
import { RepeaterRows } from '@resources/components/ui/RepeaterRows';
import type { VolumeMount } from '../hooks/useCreateContainer';
import { EMPTY_VOLUME, updateList } from './createContainerFormHelpers';

type CreateContainerVolumesSectionProps = {
  volumes: VolumeMount[];
  setVolumes: Dispatch<SetStateAction<VolumeMount[]>>;
};

export function CreateContainerVolumesSection({ volumes, setVolumes }: CreateContainerVolumesSectionProps) {
  const { t } = useTranslation('containers');
  return (
    <Section
      icon={<IconContainer className="size-3.5" />}
      title={t('create.form.volumes')}
      help={t('create.form.volumesHelp')}
    >
      <RepeaterRows<VolumeMount>
        items={volumes}
        onChange={setVolumes}
        onAdd={() => ({ ...EMPTY_VOLUME })}
        addLabel={t('create.form.addVolume')}
        renderRow={(row, _idx, { id, remove }) => (
          <Flex gap={2} align="center">
            <Input
              value={row.host}
              onChange={(e) => updateList(setVolumes, id, { ...row, host: e.target.value })}
              placeholder={t('create.form.volumesHost')}
              aria-label={t('create.form.volumesHost')}
              className="flex-1 font-mono"
            />
            <Caption className="text-muted-foreground">:</Caption>
            <Input
              value={row.container}
              onChange={(e) => updateList(setVolumes, id, { ...row, container: e.target.value })}
              placeholder={t('create.form.volumesContainer')}
              aria-label={t('create.form.volumesContainer')}
              className="flex-1 font-mono"
            />
            <Select
              value={row.mode}
              onChange={(v) => updateList(setVolumes, id, { ...row, mode: v as 'rw' | 'ro' })}
              options={[
                { value: 'rw', label: 'rw' },
                { value: 'ro', label: 'ro' },
              ]}
              className="w-20"
              searchable={false}
              ariaLabel={t('create.form.volumesMode')}
            />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('repeater.removeRowLabel', { ns: 'common' })}
              onClick={remove}
            >
              <IconTrash className="size-4 text-muted-foreground" />
            </Button>
          </Flex>
        )}
      />
    </Section>
  );
}
