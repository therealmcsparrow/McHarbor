// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDeviceDesktop } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import type { VM } from '../../types';
import { BootResolutionField } from './BootResolutionField';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

export function PlatformSection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const { draft, baseline, set } = form;
  const locked = form.locked('platform');

  return (
    <SettingsGroup
      className={className}
      icon={IconDeviceDesktop}
      title={t('settings.sections.platform')}
      description={t('settings.sections.platformDescription')}
      scope={locked ? t('settings.needsStop') : undefined}
      actions={<SaveActions vm={vm} form={form} group="platform" />}
    >
      <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2">
        <SettingsField label={t('settings.fields.machineType')}>
          <Input value={draft.machineType} disabled={locked} onChange={(evt) => set('machineType', evt.target.value)} />
        </SettingsField>

        <SettingsField
          label={t('settings.fields.firmware')}
          hint={draft.firmware !== baseline.firmware ? t('settings.firmwareWarning') : undefined}
        >
          <Select
            value={draft.firmware}
            ariaLabel={t('settings.fields.firmware')}
            disabled={locked}
            options={[
              // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: firmware standard name
              { value: 'uefi', label: 'UEFI' },
              // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: firmware standard name
              { value: 'bios', label: 'BIOS' },
            ]}
            onChange={(value) => set('firmware', value)}
          />
        </SettingsField>

        <SettingsField label={t('settings.fields.secureBoot')} description={t('create.resources.secureBootHint')}>
          <Switch
            checked={draft.secureBoot}
            disabled={locked || draft.firmware !== 'uefi'}
            aria-label={t('settings.fields.secureBoot')}
            onCheckedChange={(value) => set('secureBoot', value)}
          />
        </SettingsField>

        <SettingsField label={t('settings.fields.tpmEnabled')} description={t('create.resources.tpmHint')}>
          <Switch
            checked={draft.tpmEnabled}
            disabled={locked}
            aria-label={t('settings.fields.tpmEnabled')}
            onCheckedChange={(value) => set('tpmEnabled', value)}
          />
        </SettingsField>

        <SettingsField label={t('settings.fields.displayType')}>
          <Select
            value={draft.displayType}
            ariaLabel={t('settings.fields.displayType')}
            disabled={locked}
            options={[
              { value: '', label: t('settings.fields.displayAuto') },
              // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: QEMU display device name
              { value: 'std', label: 'std' },
              // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: QEMU display device name
              { value: 'qxl', label: 'qxl' },
              // eslint-disable-next-line mcharbor/no-restricted-syntax -- do-not-translate: QEMU display device name
              { value: 'virtio-gpu', label: 'virtio-gpu' },
              { value: 'none', label: t('settings.fields.displayNone') },
            ]}
            onChange={(value) => set('displayType', value)}
          />
        </SettingsField>

        <BootResolutionField draft={draft} locked={locked} set={set} />
      </div>
    </SettingsGroup>
  );
}
