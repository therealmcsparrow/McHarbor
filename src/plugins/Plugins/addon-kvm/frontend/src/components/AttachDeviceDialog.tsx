// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { VM_PERM } from '../permissions';
import { useAttachDevice, useHostDevices } from '../hooks/useVMs';
import { useHostFeature } from './HostFeatureGate';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import type { VM } from '../types';
import { AttachDevicePicker } from './AttachDevicePicker';

type Kind = 'pci' | 'usb' | 'block' | 'u2f';

export function AttachDeviceDialog({ vm, onClose }: { vm: VM; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const attach = useAttachDevice();

  const [kind, setKind] = useState<Kind>('pci');
  const [address, setAddress] = useState('');
  const [description, setDescription] = useState('');
  const [manual, setManual] = useState(false);

  const u2f = useHostFeature(vm.environmentId, VM_FEATURE.USB_U2F);
  // Only the kinds that name a host device need the host's inventory; an
  // emulated key and a raw /dev path do not.
  const devices = useHostDevices(vm.environmentId, kind !== 'block' && kind !== 'u2f');

  const options = useMemo(() => {
    const found = (devices.data ?? []).filter((device) => device.kind === kind);
    return found.map((device) => {
      const base = `${device.address} — ${device.description ?? ''}`.trim();
      const attached = (device.attachedTo ?? []).length > 0;
      return {
        value: device.address,
        label: !device.assignable
          ? `${base} (${t('hardware.attachDevice.unavailable')})`
          : attached
            ? `${base} (${t('hardware.attachDevice.attachedBadge')})`
            : base,
        disabled: !device.assignable,
      };
    });
  }, [devices.data, kind, t]);

  const chosen = (devices.data ?? []).find((device) => device.kind === kind && device.address === address);
  const holders = chosen?.attachedTo ?? [];
  const attachedHere = holders.some((holder) => holder.vmId === vm.id);
  const elsewhere = holders.filter((holder) => holder.vmId !== vm.id).map((holder) => holder.name);

  const pickDevice = (value: string) => {
    setAddress(value);
    const match = (devices.data ?? []).find((device) => device.address === value);
    if (match && !description.trim()) setDescription(match.description ?? '');
  };

  const hints: Record<Kind, string> = {
    pci: t('hardware.attachDevice.pciHint'),
    usb: t('hardware.attachDevice.usbHint'),
    block: t('hardware.attachDevice.blockHint'),
    u2f: t('hardware.attachDevice.u2fHint'),
  };

  const submit = () => {
    // An emulated key has no host device behind it, so it is the one kind that
    // attaches without an address.
    if (kind !== 'u2f' && !address.trim()) return;
    attach.mutate(
      {
        id: vm.id,
        kind,
        hostAddress: kind === 'u2f' ? '' : address.trim(),
        description: description.trim() || undefined,
      },
      { onSuccess: onClose },
    );
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('hardware.attachDevice.title')}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
            <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <p className="text-sm text-foreground">{t('hardware.attachDevice.warning')}</p>
          </div>

          <div>
            <Label>{t('hardware.attachDevice.kind')}</Label>
            <Select
              value={kind}
              ariaLabel={t('hardware.attachDevice.kind')}
              options={[
                { value: 'pci', label: t('hardware.attachDevice.pci') },
                { value: 'usb', label: t('hardware.attachDevice.usb') },
                { value: 'block', label: t('hardware.attachDevice.block') },
                {
                  value: 'u2f',
                  label: t('hardware.attachDevice.u2f'),
                  // Select has no disabled option, so the reason rides on the
                  // row's second line and the submit button is what actually
                  // refuses — the operator still sees the choice and why it
                  // will not work here.
                  description: u2f.supported ? undefined : u2f.reason,
                },
              ]}
              onChange={(value) => {
                setKind(value as Kind);
                setAddress('');
              }}
            />
          </div>

          {kind === 'u2f' && <p className="text-sm text-muted-foreground">{t('hardware.attachDevice.u2fHint')}</p>}

          {kind !== 'block' && kind !== 'u2f' && !manual && (
            <AttachDevicePicker
              isLoading={devices.isLoading}
              isError={devices.isError}
              onRefresh={() => void devices.refetch()}
              options={options}
              address={address}
              onPick={pickDevice}
              chosen={chosen}
              attachedHere={attachedHere}
              elsewhere={elsewhere}
              onManual={() => setManual(true)}
            />
          )}

          {(kind === 'block' || (manual && kind !== 'u2f')) && (
            <div>
              <Label htmlFor="device-address">{t('hardware.attachDevice.address')}</Label>
              <Input
                id="device-address"
                value={address}
                autoComplete="off"
                onChange={(evt) => setAddress(evt.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">{hints[kind]}</p>
            </div>
          )}

          <div>
            <Label htmlFor="device-note">{t('hardware.attachDevice.description')}</Label>
            <Input
              id="device-note"
              value={description}
              autoComplete="off"
              onChange={(evt) => setDescription(evt.target.value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button
            variant="destructive"
            permission={VM_PERM.PRIVILEGED}
            disabled={
              (kind !== 'u2f' && !address.trim()) ||
              attach.isPending ||
              (kind === 'u2f' && !u2f.supported) ||
              chosen?.assignable === false ||
              (!manual && attachedHere)
            }
            onClick={submit}
          >
            {t('hardware.attachDevice.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
