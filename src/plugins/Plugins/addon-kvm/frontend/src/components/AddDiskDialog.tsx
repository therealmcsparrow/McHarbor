// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { VM_PERM } from '../permissions';
import { HotplugNotice } from './HotplugNotice';
import { DISK_BUSES, defaultDiskBus } from '../diskBus';
import { useAddDisk } from '../hooks/useVMs';
import { useComputePools } from '../hooks/useComputeResources';
import type { VM } from '../types';

export function AddDiskDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const addDisk = useAddDisk();
  const { data: pools } = useComputePools(vm.environmentId);

  const [pickedPoolId, setPoolId] = useState('');
  const [notice, setNotice] = useState('');
  const [sizeGb, setSizeGb] = useState(20);
  const [bus, setBus] = useState(defaultDiskBus(vm.guestOsFamily));

  const poolId = pickedPoolId || (pools?.length ? (pools.find((entry) => entry.isDefault) ?? pools[0])!.id : '');

  const pool = pools?.find((entry) => entry.id === poolId);

  const submit = () => {
    if (!pool) return;
    addDisk.mutate(
      {
        id: vm.id,
        poolId: pool.id,
        poolBackend: pool.backend,
        poolTarget: pool.target,
        sizeGb,
        bus,
      },
      { onSuccess: (added) => (added.notice ? setNotice(added.notice) : onClose()) },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('hardware.addDisk.title')}</DialogTitle>
          <DialogDescription>
            {vm.state === 'running' ? t('hardware.liveHint') : t('hardware.mustBeStopped')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <Label>{t('hardware.addDisk.pool')}</Label>
            {pools && pools.length === 0 ? (
              <p className="text-sm text-warning">{t('hardware.addDisk.noPools')}</p>
            ) : (
              <Select
                value={poolId}
                ariaLabel={t('hardware.addDisk.pool')}
                options={(pools ?? []).map((entry) => ({
                  value: entry.id,
                  label: `${entry.name} (${entry.backend})`,
                }))}
                onChange={setPoolId}
              />
            )}
          </div>

          <div>
            <Label htmlFor="disk-size">{t('hardware.addDisk.size')}</Label>
            <NumberInput id="disk-size" min={1} value={sizeGb} onChange={setSizeGb} />
          </div>

          <div>
            <Label>{t('hardware.addDisk.bus')}</Label>
            <Select
              value={bus}
              ariaLabel={t('hardware.addDisk.bus')}
              options={DISK_BUSES.map((entry) => ({ value: entry, label: entry }))}
              onChange={setBus}
            />
            {vm.guestOsFamily === 'windows' && bus.startsWith('virtio') && (
              <p className="mt-1 text-xs text-warning">{t('hardware.addDisk.virtioWindows')}</p>
            )}
          </div>
        </div>

        <HotplugNotice notice={notice} />

        <DialogFooter>
          {notice ? (
            <Button onClick={onClose}>{t('actions.close', { ns: 'common' })}</Button>
          ) : (
            <>
              <Button variant="ghost" onClick={onClose}>
                {t('actions.cancel')}
              </Button>
              <Button permission={VM_PERM.MANAGE} disabled={!pool || addDisk.isPending} onClick={submit}>
                {t('hardware.addDisk.submit')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
