// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { useHostDevices, type HostDevice } from '../../hooks/useHostDevices';
import { buildDeviceOptions, findHostDevice, MANUAL_DEVICE_VALUE } from './host-device-options';

const DEVICE_PATH_EXAMPLE = '/dev/ttyUSB0';

type HostDevicePathFieldProps = {
  id: string;
  label: string;
  value: string;
  onChange: (value: string, device: HostDevice | undefined) => void;
};

export function HostDevicePathField({ id, label, value, onChange }: HostDevicePathFieldProps) {
  const { t } = useTranslation('containers');
  const { data, isLoading, isError, error, refetch, allowed } = useHostDevices();
  const [manualChosen, setManualChosen] = useState(false);
  const devices = useMemo(() => data?.devices ?? [], [data]);
  const options = useMemo(() => buildDeviceOptions(devices, t), [devices, t]);
  const selected = findHostDevice(devices, value);

  if (allowed && isLoading) {
    return (
      <div className="min-w-0">
        <Label className="mb-1 text-xs font-medium text-muted-foreground">{label}</Label>
        <Select
          variant="outline"
          ariaLabel={label}
          value=""
          onChange={() => undefined}
          options={[]}
          placeholder={t('resources.hostDevices.loading')}
          disabled
        />
      </div>
    );
  }

  const listUsable = allowed && !isError && devices.length > 0;
  const manual = manualChosen || !listUsable || (value !== '' && selected === undefined);

  if (manual) {
    return (
      <div className="min-w-0">
        <Label htmlFor={id} className="mb-1 text-xs font-medium text-muted-foreground">
          {label}
        </Label>
        <div className="flex items-center gap-2">
          <Input
            id={id}
            variant="outline"
            className="font-mono text-xs"
            value={value}
            onChange={(evt) => onChange(evt.target.value, undefined)}
            placeholder={DEVICE_PATH_EXAMPLE}
          />
          {(listUsable || !allowed) && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              permission={PERM.HOST_VIEW}
              onClick={() => {
                setManualChosen(false);
                if (selected === undefined) onChange('', undefined);
              }}
            >
              {t('resources.hostDevices.chooseFromList')}
            </Button>
          )}
        </div>
        {allowed && isError && (
          <div className="mt-1 flex items-center gap-2 text-[10px] text-danger">
            <span>{[t('resources.hostDevices.loadFailed'), error?.message].filter(Boolean).join(' — ')}</span>
            <Button type="button" variant="link" size="sm" onClick={() => void refetch()}>
              {t('errorState.retry', { ns: 'common' })}
            </Button>
          </div>
        )}
        {allowed && !isError && devices.length === 0 && (
          <p className="mt-1 text-[10px] text-muted-foreground/70">{t('resources.hostDevices.empty')}</p>
        )}
      </div>
    );
  }

  return (
    <div className="min-w-0">
      <Label className="mb-1 text-xs font-medium text-muted-foreground">{label}</Label>
      <Select
        variant="outline"
        ariaLabel={label}
        value={value}
        onChange={(next) => {
          if (next === MANUAL_DEVICE_VALUE) {
            setManualChosen(true);
            return;
          }
          onChange(next, findHostDevice(devices, next));
        }}
        options={options}
        placeholder={t('resources.hostDevices.placeholder')}
      />
      {selected && selected.stablePath && (
        <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground/70">
          {t('resources.hostDevices.resolvesTo', { path: selected.path })}
        </p>
      )}
      {selected?.dangerous && (
        <div className="mt-1 flex items-start gap-2">
          <Badge variant="warning">{t('resources.hostDevices.blockBadge')}</Badge>
          <span className="text-[10px] text-warning">{t('resources.hostDevices.blockWarning')}</span>
        </div>
      )}
      {data?.truncated && (
        <p className="mt-1 text-[10px] text-muted-foreground/70">{t('resources.hostDevices.truncated')}</p>
      )}
    </div>
  );
}
