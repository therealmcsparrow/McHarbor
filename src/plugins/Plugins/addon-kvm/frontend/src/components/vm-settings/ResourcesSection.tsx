// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCpu } from '@tabler/icons-react';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { formatBytes } from '../../format';
import { useQEMUCapability } from '../../hooks/useQEMUCapability';
import type { VM } from '../../types';
import { MemoryField, VcpuField } from './ResourceSliders';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

export function ResourcesSection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const { data: qemu } = useQEMUCapability(vm.environmentId);
  const { draft, set, limits } = form;
  const locked = form.locked('resources');

  // `host` first and named as the trade it is. An operator who picks it gets
  // every instruction the physical CPU has and a guest that cannot migrate,
  // and the picker is the one place that fact will actually be read.
  const cpuModelOptions = useMemo(() => {
    const models = qemu?.cpuModels ?? [];
    return [
      { value: 'host', label: t('settings.fields.cpuModelHost') },
      ...models
        .filter((modelValue) => modelValue !== 'host')
        .map((modelValue) => ({ value: modelValue, label: modelValue })),
    ];
  }, [qemu?.cpuModels, t]);

  return (
    <SettingsGroup
      className={className}
      icon={IconCpu}
      title={t('settings.sections.resources')}
      description={
        limits.known
          ? t('settings.hostSummary', {
              cpus: limits.hostCpus,
              memory: formatBytes(limits.hostMemoryMb * 1024 * 1024),
            })
          : t('settings.hostUnknown')
      }
      scope={locked ? t('settings.needsStop') : undefined}
      actions={<SaveActions vm={vm} form={form} group="resources" />}
    >
      <div className="grid gap-x-6 gap-y-5 lg:grid-cols-2">
        <VcpuField form={form} />

        <SettingsField label={t('settings.fields.cpuModel')} description={t('settings.fields.cpuModelHint')}>
          <Select
            value={draft.cpuModel || 'host'}
            ariaLabel={t('settings.fields.cpuModel')}
            disabled={locked}
            searchable
            options={cpuModelOptions}
            onChange={(value) => set('cpuModel', value === 'host' ? '' : value)}
          />
        </SettingsField>

        <MemoryField form={form} />

        <SettingsField label={t('settings.fields.memoryMaxMb')} description={t('settings.fields.memoryMaxHint')}>
          <NumberInput
            value={draft.memoryMaxMb}
            min={0}
            step={128}
            className="w-40"
            disabled={locked}
            onChange={(value) => set('memoryMaxMb', value)}
          />
        </SettingsField>
      </div>
    </SettingsGroup>
  );
}
