// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';

export function CreateResourcesStep({ form }: { form: CreateVMForm }) {
  const { t } = useTranslation('vm');
  const {
    preset,
    vcpus,
    setVcpus,
    memoryMb,
    setMemoryMb,
    firmware,
    setFirmware,
    secureBoot,
    setSecureBoot,
    tpmEnabled,
    setTpmEnabled,
    applyPreset,
    swtpm,
    tpmAvailable,
    secboot,
    secureBootAvailable,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.resources.title')}</h3>

      <div>
        <Label>{t('create.resources.preset')}</Label>
        <Select
          value={preset}
          ariaLabel={t('create.resources.preset')}
          options={[
            {
              value: 'small',
              label: t('create.resources.presets.small'),
            },
            {
              value: 'medium',
              label: t('create.resources.presets.medium'),
            },
            {
              value: 'large',
              label: t('create.resources.presets.large'),
            },
            {
              value: 'custom',
              label: t('create.resources.presets.custom'),
            },
          ]}
          onChange={applyPreset}
        />
      </div>

      {preset === 'custom' && (
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <Label htmlFor="vm-vcpus">{t('create.resources.vcpus')}</Label>
            <NumberInput id="vm-vcpus" min={1} max={128} value={vcpus} onChange={setVcpus} />
          </div>
          <div>
            <Label htmlFor="vm-memory">{t('create.resources.memory')}</Label>
            <NumberInput id="vm-memory" min={128} step={256} value={memoryMb} onChange={setMemoryMb} />
          </div>
        </div>
      )}

      <div>
        <Label>{t('create.resources.firmware')}</Label>
        <Select
          value={firmware}
          ariaLabel={t('create.resources.firmware')}
          options={[
            { value: 'uefi', label: t('create.resources.uefi') },
            { value: 'bios', label: t('create.resources.bios') },
          ]}
          onChange={(value) => setFirmware(value as 'uefi' | 'bios')}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.resources.uefiHint')}</p>
      </div>

      {firmware === 'uefi' && (
        <>
          <div className="flex items-start justify-between gap-3">
            <div>
              <Label htmlFor="vm-secureboot" className="text-sm font-normal">
                {t('create.resources.secureBoot')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {secureBootAvailable
                  ? t('create.resources.secureBootHint')
                  : t('create.resources.secureBootMissing', {
                      remediation: secboot?.remediation ?? 'ovmf',
                    })}
              </p>
            </div>
            <Switch
              id="vm-secureboot"
              checked={secureBoot && secureBootAvailable}
              disabled={!secureBootAvailable}
              onCheckedChange={setSecureBoot}
            />
          </div>

          <div className="flex items-start justify-between gap-3">
            <div>
              <Label htmlFor="vm-tpm" className="text-sm font-normal">
                {t('create.resources.tpm')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {tpmAvailable
                  ? t('create.resources.tpmHint')
                  : t('create.resources.tpmMissing', {
                      remediation: swtpm?.remediation ?? 'swtpm',
                    })}
              </p>
            </div>
            <Switch
              id="vm-tpm"
              checked={tpmEnabled && tpmAvailable}
              disabled={!tpmAvailable}
              onCheckedChange={setTpmEnabled}
            />
          </div>
        </>
      )}
    </>
  );
}
