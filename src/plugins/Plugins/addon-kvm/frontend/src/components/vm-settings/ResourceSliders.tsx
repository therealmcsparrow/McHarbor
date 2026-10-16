// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Slider } from '@resources/components/ui/Slider';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import { formatBytes } from '../../format';
import type { VmSettingsForm } from './useVmSettingsForm';

export function VcpuField({ form }: { form: VmSettingsForm }) {
  const { t } = useTranslation('vm');
  const { draft, set, limits, zones } = form;
  const locked = form.locked('resources');

  return (
    <SettingsField
      label={t('settings.fields.vcpus')}
      description={
        limits.known
          ? t('settings.cpuNote', {
              cores: limits.physicalCores,
              threads: limits.hostCpus,
              ideal: limits.idealMaxVCPUs,
              safe: limits.safeVCPUs,
            })
          : undefined
      }
      hint={
        limits.cpusOverBudget
          ? t('settings.overBudget', {
              value: String(draft.vcpus),
              max: String(limits.maxVCPUs),
            })
          : limits.cpusOverSafe
            ? t('settings.cpuOverSafe', {
                value: String(draft.vcpus),
                safe: String(limits.safeVCPUs),
                others: String(limits.otherAllocatedVCPUs),
              })
            : undefined
      }
    >
      {limits.maxVCPUs > 1 ? (
        <Slider
          value={Math.min(draft.vcpus, limits.maxVCPUs)}
          min={1}
          max={limits.maxVCPUs}
          disabled={locked}
          valueLabel={t('settings.fields.vcpusValue', {
            count: draft.vcpus,
          })}
          minLabel="1"
          maxLabel={String(limits.maxVCPUs)}
          zones={zones}
          aria-label={t('settings.fields.vcpus')}
          onChange={(value) => set('vcpus', value)}
        />
      ) : (
        <NumberInput
          value={draft.vcpus}
          min={1}
          className="w-40"
          disabled={locked}
          onChange={(value) => set('vcpus', value)}
        />
      )}
    </SettingsField>
  );
}

export function MemoryField({ form }: { form: VmSettingsForm }) {
  const { t } = useTranslation('vm');
  const { draft, set, limits } = form;
  const locked = form.locked('resources');

  return (
    <SettingsField
      label={t('settings.fields.memoryMb')}
      description={
        limits.known
          ? limits.otherAllocatedMb > 0
            ? t('settings.memoryNoteShared', {
                reserve: formatBytes(limits.reserveMb * 1024 * 1024),
                used: formatBytes(limits.otherAllocatedMb * 1024 * 1024),
              })
            : t('settings.memoryNote', {
                reserve: formatBytes(limits.reserveMb * 1024 * 1024),
              })
          : undefined
      }
      hint={
        limits.memoryOverBudget
          ? t('settings.overBudget', {
              value: formatBytes(draft.memoryMb * 1024 * 1024),
              max: formatBytes(limits.maxMemoryMb * 1024 * 1024),
            })
          : undefined
      }
    >
      {limits.maxMemoryMb > limits.memoryStepMb ? (
        <Slider
          value={Math.min(draft.memoryMb, limits.maxMemoryMb)}
          min={limits.memoryStepMb}
          max={limits.maxMemoryMb}
          step={limits.memoryStepMb}
          disabled={locked}
          valueLabel={formatBytes(draft.memoryMb * 1024 * 1024)}
          minLabel={formatBytes(limits.memoryStepMb * 1024 * 1024)}
          maxLabel={formatBytes(limits.maxMemoryMb * 1024 * 1024)}
          aria-label={t('settings.fields.memoryMb')}
          onChange={(value) => set('memoryMb', value)}
        />
      ) : (
        <NumberInput
          value={draft.memoryMb}
          min={128}
          step={128}
          className="w-40"
          disabled={locked}
          onChange={(value) => set('memoryMb', value)}
        />
      )}
    </SettingsField>
  );
}
