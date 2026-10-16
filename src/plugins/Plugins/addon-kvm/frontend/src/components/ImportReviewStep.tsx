// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { ImportForm } from '../hooks/useImportForm';
import type { ImportInspection } from '../types';
import { formatBytes } from '../format';

export function ImportReviewStep({ form, found }: { form: ImportForm; found: ImportInspection }) {
  const { t } = useTranslation('vm');
  const {
    job,
    nameValue,
    setName,
    poolId,
    setPoolId,
    networkId,
    setNetworkId,
    vcpus,
    setVcpus,
    memoryMb,
    setMemoryMb,
    firmware,
    setFirmware,
    guestOs,
    setGuestOs,
    secureBoot,
    setSecureBoot,
    tpmEnabled,
    setTpmEnabled,
    autostart,
    setAutostart,
    pools,
    networks,
  } = form;

  return (
    <div className="grid gap-4">
      {found.warnings?.length ? (
        <Alert
          variant="warning"
          icon={<IconAlertTriangle className="size-4 shrink-0" aria-hidden="true" />}
          title={t('import.warnings')}
          description={found.warnings.join(' ')}
        />
      ) : null}

      <div className="rounded-md border border-border p-3">
        <p className="mb-2 text-sm font-medium text-foreground">{t('import.disks')}</p>
        <ul className="space-y-1">
          {(found.disks ?? []).map((disk) => (
            <li key={disk.index} className="text-xs text-muted-foreground">
              {disk.sourceFormat} · {formatBytes(disk.virtualSize)}
              {disk.index === 0 ? ` · ${t('import.bootDisk')}` : ''}
            </li>
          ))}
        </ul>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-name">{t('import.field.name')}</Label>
        <Input id="import-name" value={nameValue} onChange={(evt) => setName(evt.target.value)} />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-pool">{t('import.field.pool')}</Label>
        <Select
          value={poolId}
          onChange={setPoolId}
          options={(pools.data ?? []).map((entry) => ({
            value: entry.id,
            label: `${entry.name} (${entry.backend})`,
          }))}
          placeholder={t('import.field.poolPlaceholder')}
          ariaLabel={t('import.field.pool')}
        />
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-network">{t('import.field.network')}</Label>
        <Select
          value={networkId}
          onChange={setNetworkId}
          options={(networks.data ?? []).map((entry) => ({
            value: entry.id,
            label: entry.name,
          }))}
          placeholder={t('import.field.networkPlaceholder')}
          ariaLabel={t('import.field.network')}
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor="import-vcpus">{t('import.field.vcpus')}</Label>
          <NumberInput
            id="import-vcpus"
            nullable
            min={1}
            value={vcpus || null}
            onChange={(next) => setVcpus(next ?? 0)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="import-memory">{t('import.field.memory')}</Label>
          <NumberInput
            id="import-memory"
            nullable
            min={256}
            value={memoryMb || null}
            onChange={(next) => setMemoryMb(next ?? 0)}
          />
        </div>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-guest-os">{t('import.field.guestOs')}</Label>
        <Select
          value={guestOs || 'linux'}
          onChange={setGuestOs}
          options={[
            { value: 'linux', label: t('import.guestOs.linux') },
            { value: 'windows', label: t('import.guestOs.windows') },
            { value: 'bsd', label: t('import.guestOs.bsd') },
            { value: 'other', label: t('import.guestOs.other') },
          ]}
          ariaLabel={t('import.field.guestOs')}
        />
        <p className="text-xs text-muted-foreground">{t('import.field.guestOsHelp')}</p>
      </div>

      <div className="grid gap-1.5">
        <Label htmlFor="import-firmware">{t('import.field.firmware')}</Label>
        <Select
          value={firmware || 'bios'}
          onChange={setFirmware}
          options={[
            { value: 'bios', label: t('import.firmware.bios') },
            { value: 'uefi', label: t('import.firmware.uefi') },
          ]}
          ariaLabel={t('import.field.firmware')}
        />
        <p className="text-xs text-muted-foreground">{t('import.field.firmwareHelp')}</p>
      </div>

      <div className="flex items-center justify-between gap-3">
        <div className="grid gap-0.5">
          <Label htmlFor="import-secure-boot">{t('import.field.secureBoot')}</Label>
          <p className="text-xs text-muted-foreground">{t('import.field.secureBootHelp')}</p>
        </div>
        <Switch
          id="import-secure-boot"
          checked={secureBoot}
          disabled={firmware !== 'uefi'}
          onCheckedChange={setSecureBoot}
        />
      </div>

      <div className="flex items-center justify-between gap-3">
        <div className="grid gap-0.5">
          <Label htmlFor="import-tpm">{t('import.field.tpm')}</Label>
          <p className="text-xs text-muted-foreground">{t('import.field.tpmHelp')}</p>
        </div>
        <Switch id="import-tpm" checked={tpmEnabled} onCheckedChange={setTpmEnabled} />
      </div>

      <div className="flex items-center justify-between gap-3">
        <Label htmlFor="import-autostart">{t('import.field.autostart')}</Label>
        <Switch id="import-autostart" checked={autostart} onCheckedChange={setAutostart} />
      </div>

      {job.phase === 'converting' ? <p className="text-xs text-muted-foreground">{t('import.converting')}</p> : null}
    </div>
  );
}
