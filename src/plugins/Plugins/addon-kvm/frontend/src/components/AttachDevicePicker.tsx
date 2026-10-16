// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { HostDevice } from '../types';

export function AttachDevicePicker({
  isLoading,
  isError,
  onRefresh,
  options,
  address,
  onPick,
  chosen,
  attachedHere,
  elsewhere,
  onManual,
}: {
  isLoading: boolean;
  isError: boolean;
  onRefresh: () => void;
  options: { value: string; label: string; disabled?: boolean }[];
  address: string;
  onPick: (value: string) => void;
  chosen: HostDevice | undefined;
  attachedHere: boolean;
  elsewhere: string[];
  onManual: () => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <div>
      <div className="flex items-center justify-between">
        <Label>{t('hardware.attachDevice.selectDevice')}</Label>
        <Button variant="ghost" size="sm" onClick={onRefresh}>
          {t('hardware.attachDevice.refresh')}
        </Button>
      </div>
      {isLoading ? (
        <p className="text-xs text-muted-foreground">{t('hardware.attachDevice.loading')}</p>
      ) : isError ? (
        <p className="text-xs text-danger">{t('hardware.attachDevice.loadFailed')}</p>
      ) : options.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('hardware.attachDevice.none')}</p>
      ) : (
        <Select
          value={address}
          ariaLabel={t('hardware.attachDevice.selectDevice')}
          placeholder={t('hardware.attachDevice.selectPlaceholder')}
          options={options}
          onChange={onPick}
        />
      )}
      {chosen && !chosen.assignable && (
        <p className="mt-1 text-xs text-warning">
          {chosen.reason === 'no_iommu_group'
            ? t('hardware.attachDevice.reason_no_iommu_group')
            : t('hardware.attachDevice.reason_bound_to_host_driver', {
                driver: chosen.driver ?? '',
              })}
        </p>
      )}
      {attachedHere && <p className="mt-1 text-xs text-warning">{t('hardware.attachDevice.attachedHere')}</p>}
      {!attachedHere && elsewhere.length > 0 && (
        <p className="mt-1 text-xs text-warning">
          {t('hardware.attachDevice.attachedElsewhere', {
            names: elsewhere.join(', '),
          })}
        </p>
      )}
      {chosen?.iommuGroup && (
        <p className="mt-1 text-xs text-muted-foreground">
          {t('hardware.attachDevice.iommuGroup', {
            group: chosen.iommuGroup,
          })}
        </p>
      )}
      <Button variant="ghost" size="sm" className="mt-1" onClick={onManual}>
        {t('hardware.attachDevice.manualEntry')}
      </Button>
    </div>
  );
}
