// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { buildPermissionOptions } from './host-device-options';

type DevicePermissionsFieldProps = {
  label: string;
  value: string;
  onChange: (value: string) => void;
};

export function DevicePermissionsField({ label, value, onChange }: DevicePermissionsFieldProps) {
  const { t } = useTranslation('containers');
  const current = value.trim() || 'rwm';
  return (
    <div className="min-w-0">
      <Label className="mb-1 text-xs font-medium text-muted-foreground">{label}</Label>
      <Select
        variant="outline"
        ariaLabel={label}
        value={current}
        onChange={onChange}
        options={buildPermissionOptions(current, t)}
        searchable={false}
      />
      <p className="mt-1 text-[10px] text-muted-foreground/70">{t('resources.hostDevices.permissionsHelp')}</p>
    </div>
  );
}
