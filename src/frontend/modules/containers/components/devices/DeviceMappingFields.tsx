// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { DeviceMappingEntry } from '../../types/edit-form';
import { useHostDevices, type HostDevice } from '../../hooks/useHostDevices';
import { DevicePermissionsField } from './DevicePermissionsField';
import { HostDevicePathField } from './HostDevicePathField';
import { findHostDevice, freeContainerPath, nextContainerPath } from './host-device-options';

const DEVICE_PATH_EXAMPLE = '/dev/ttyUSB0';

type DeviceMappingFieldsProps = {
  idPrefix: string;
  value: DeviceMappingEntry;
  onChange: (next: DeviceMappingEntry) => void;
  onRemove: () => void;
  takenContainerPaths?: string[];
};

export function DeviceMappingFields({
  idPrefix,
  value,
  onChange,
  onRemove,
  takenContainerPaths = [],
}: DeviceMappingFieldsProps) {
  const { t } = useTranslation('containers');
  const { data } = useHostDevices();
  const devices = data?.devices ?? [];

  const handleHostPath = (hostPath: string, device: HostDevice | undefined) => {
    const proposed = nextContainerPath(
      value.pathInContainer,
      value.pathOnHost,
      findHostDevice(devices, value.pathOnHost),
      hostPath,
      device,
    );
    const changed = proposed !== value.pathInContainer;
    onChange({
      ...value,
      pathOnHost: hostPath,
      pathInContainer: changed && proposed ? freeContainerPath(proposed, takenContainerPaths) : proposed,
    });
  };

  const containerPath = value.pathInContainer.trim();
  const taken = containerPath !== '' && takenContainerPaths.some((path) => path.trim() === containerPath);
  const freePath = taken ? freeContainerPath(containerPath, takenContainerPaths) : '';

  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_150px_auto]">
      <HostDevicePathField
        id={`${idPrefix}-host`}
        label={t('resources.deviceHostPath')}
        value={value.pathOnHost}
        onChange={handleHostPath}
      />
      <div className="min-w-0">
        <Label htmlFor={`${idPrefix}-container`} className="mb-1 text-xs font-medium text-muted-foreground">
          {t('resources.deviceContainerPath')}
        </Label>
        <Input
          id={`${idPrefix}-container`}
          variant="outline"
          className="font-mono text-xs"
          value={value.pathInContainer}
          onChange={(evt) => onChange({ ...value, pathInContainer: evt.target.value })}
          placeholder={findHostDevice(devices, value.pathOnHost)?.path ?? DEVICE_PATH_EXAMPLE}
        />
        {taken && (
          <div className="mt-1 flex flex-wrap items-center gap-x-2 text-[10px] text-warning">
            <span>{t('resources.hostDevices.pathTaken')}</span>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto p-0 text-[10px]"
              onClick={() => onChange({ ...value, pathInContainer: freePath })}
            >
              {t('resources.hostDevices.useFreePath', { path: freePath })}
            </Button>
          </div>
        )}
      </div>
      <DevicePermissionsField
        label={t('resources.devicePermissions')}
        value={value.cgroupPermissions}
        onChange={(cgroupPermissions) => onChange({ ...value, cgroupPermissions })}
      />
      <div className="flex items-start pt-5">
        <Button type="button" variant="ghost" size="icon" aria-label={t('resources.removeDevice')} onClick={onRemove}>
          <IconTrash className="size-4" />
        </Button>
      </div>
    </div>
  );
}
