// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconUsb } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Section } from '@resources/components/ui/Form';
import type { DeviceMappingEntry } from '../types/edit-form';
import { DeviceMappingFields } from './devices/DeviceMappingFields';
import { useRowKeys } from './devices/useRowKeys';

type CreateContainerDevicesSectionProps = {
  value: DeviceMappingEntry[];
  onChange: (next: DeviceMappingEntry[]) => void;
};

const EMPTY_DEVICE: DeviceMappingEntry = { pathOnHost: '', pathInContainer: '', cgroupPermissions: 'rwm' };

export function CreateContainerDevicesSection({ value, onChange }: CreateContainerDevicesSectionProps) {
  const { t } = useTranslation('containers');
  const { keyAt, forget } = useRowKeys(value.length, 'create-device');

  return (
    <Section
      icon={<IconUsb className="size-3.5" />}
      title={t('overview.devices')}
      help={t('resources.hostDevices.sectionHelp')}
    >
      <div className="space-y-3">
        {value.map((device, index) => (
          <div key={keyAt(index)} className="rounded-md border border-border bg-muted/20 p-3">
            <DeviceMappingFields
              idPrefix={keyAt(index)}
              value={device}
              onChange={(next) => onChange(value.map((row, current) => (current === index ? next : row)))}
              takenContainerPaths={value.filter((_, current) => current !== index).map((row) => row.pathInContainer)}
              onRemove={() => {
                forget(index);
                onChange(value.filter((_, current) => current !== index));
              }}
            />
          </div>
        ))}
        <Button type="button" variant="ghost" size="sm" onClick={() => onChange([...value, { ...EMPTY_DEVICE }])}>
          <IconPlus className="size-4" />
          {t('resources.addDevice')}
        </Button>
        <p className="text-[10px] text-muted-foreground/70">{t('resources.devicesHelp')}</p>
      </div>
    </Section>
  );
}
