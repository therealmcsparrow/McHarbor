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
import { Select } from '@resources/components/ui/Select';
import { VM_PERM } from '../permissions';
import { HotplugNotice } from './HotplugNotice';
import { useAddNIC } from '../hooks/useVMs';
import { useComputeNetworks } from '../hooks/useComputeResources';
import type { VM } from '../types';
import { nicAttachment } from '../utils/nicAttachment';

const MODELS = ['virtio-net', 'e1000', 'rtl8139'];

export function AddNICDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const addNIC = useAddNIC();
  const { data: networks } = useComputeNetworks(vm.environmentId);

  const [pickedNetworkId, setNetworkId] = useState('');

  const [notice, setNotice] = useState('');
  const [modelValue, setModel] = useState('virtio-net');

  const networkId =
    pickedNetworkId || (networks?.length ? (networks.find((entry) => entry.isDefault) ?? networks[0])!.id : '');

  const network = networks?.find((entry) => entry.id === networkId);

  const submit = () => {
    if (!network) return;
    addNIC.mutate(
      {
        id: vm.id,
        networkId: network.id,
        ...nicAttachment(network),
        model: modelValue,
      },
      { onSuccess: (added) => (added.notice ? setNotice(added.notice) : onClose()) },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('hardware.addNic.title')}</DialogTitle>
          <DialogDescription>
            {vm.state === 'running' ? t('hardware.liveHint') : t('hardware.mustBeStopped')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div>
            <Label>{t('hardware.addNic.network')}</Label>
            {networks && networks.length === 0 ? (
              <p className="text-sm text-warning">{t('hardware.addNic.noNetworks')}</p>
            ) : (
              <Select
                value={networkId}
                ariaLabel={t('hardware.addNic.network')}
                options={(networks ?? []).map((entry) => ({
                  value: entry.id,
                  label: entry.vlanTag ? `${entry.name} · VLAN ${entry.vlanTag}` : entry.name,
                }))}
                onChange={setNetworkId}
              />
            )}
          </div>

          <div>
            <Label>{t('hardware.addNic.model')}</Label>
            <Select
              value={modelValue}
              ariaLabel={t('hardware.addNic.model')}
              options={MODELS.map((entry) => ({ value: entry, label: entry }))}
              onChange={setModel}
            />
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
              <Button permission={VM_PERM.MANAGE} disabled={!network || addNIC.isPending} onClick={submit}>
                {t('hardware.addNic.submit')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
