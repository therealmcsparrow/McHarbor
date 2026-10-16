// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCpu } from '@tabler/icons-react';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { VM_FEATURE, useQEMUCapability } from '../../hooks/useQEMUCapability';
import type { VM } from '../../types';
import { useHostFeature } from '../HostFeatureGate';
import { FeatureSwitch } from './FeatureSwitch';
import { SaveActions } from './SaveActions';
import type { VmSettingsForm } from './useVmSettingsForm';

export function FeaturesSection({ vm, form, className }: { vm: VM; form: VmSettingsForm; className?: string }) {
  const { t } = useTranslation('vm');
  const virtioMem = useHostFeature(vm.environmentId, VM_FEATURE.VIRTIO_MEM);
  const cpuHotplug = useHostFeature(vm.environmentId, VM_FEATURE.CPU_HOTPLUG);
  const hugePages = useHostFeature(vm.environmentId, VM_FEATURE.HUGEPAGES);
  const numa = useHostFeature(vm.environmentId, VM_FEATURE.NUMA);
  const ioThread = useHostFeature(vm.environmentId, VM_FEATURE.IOTHREAD);
  const vsock = useHostFeature(vm.environmentId, VM_FEATURE.VSOCK);
  const { data: qemu } = useQEMUCapability(vm.environmentId);
  const { draft, set, limits } = form;
  const locked = form.locked('features');

  // The node the guest is pinned to, so the field can say what is free on it.
  // A pin onto a node with no memory left turns a start failure into a puzzle,
  // and the number is only knowable before the start.
  const pinnedNode = useMemo(
    () => (qemu?.numaNodes ?? []).find((node) => node.id === draft.numaHostNodes[0]),
    [qemu?.numaNodes, draft.numaHostNodes],
  );

  return (
    <SettingsGroup
      className={className}
      icon={IconCpu}
      title={t('settings.sections.features')}
      description={t('settings.sections.featuresDescription')}
      scope={locked ? t('settings.needsStop') : undefined}
      actions={<SaveActions vm={vm} form={form} group="features" />}
    >
      <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2">
        <SettingsField label={t('settings.fields.vcpusMax')} description={t('settings.fields.vcpusMaxHint')}>
          <NumberInput
            value={draft.vcpusMax}
            min={0}
            max={limits.maxVCPUs}
            className="w-40"
            disabled={locked || !cpuHotplug.supported}
            onChange={(value) => set('vcpusMax', value)}
          />
        </SettingsField>

        <FeatureSwitch
          label={t('settings.fields.virtioMem')}
          description={t('settings.fields.virtioMemHint')}
          checked={draft.virtioMem}
          feature={virtioMem}
          disabled={locked}
          onChange={(value) => set('virtioMem', value)}
        />

        <FeatureSwitch
          label={t('settings.fields.hugePages')}
          description={t('settings.fields.hugePagesHint')}
          checked={draft.hugePages}
          feature={hugePages}
          disabled={locked}
          onChange={(value) => set('hugePages', value)}
        />

        <SettingsField label={t('settings.fields.numaPolicy')} description={t('settings.fields.numaPolicyHint')}>
          <Select
            value={draft.numaPolicy || 'none'}
            ariaLabel={t('settings.fields.numaPolicy')}
            disabled={locked || !numa.supported}
            options={[
              { value: 'none', label: t('settings.fields.numaNone') },
              { value: 'pin', label: t('settings.fields.numaPin') },
              { value: 'mirror', label: t('settings.fields.numaMirror') },
            ]}
            onChange={(value) => set('numaPolicy', value)}
          />
        </SettingsField>

        {draft.numaPolicy === 'pin' && (
          <SettingsField
            label={t('settings.fields.numaNode')}
            description={
              pinnedNode
                ? t('settings.fields.numaNodeFree', {
                    free: Math.round((pinnedNode.freeBytes ?? 0) / (1024 * 1024 * 1024)),
                    cpus: pinnedNode.cpus?.length ?? 0,
                  })
                : t('settings.fields.numaNodeHint')
            }
          >
            <Select
              value={String(draft.numaHostNodes[0] ?? '')}
              ariaLabel={t('settings.fields.numaNode')}
              disabled={locked || !numa.supported}
              options={(qemu?.numaNodes ?? []).map((node) => ({
                value: String(node.id),
                label: t('settings.fields.numaNodeLabel', { id: node.id }),
              }))}
              onChange={(value) => set('numaHostNodes', [Number(value)])}
            />
          </SettingsField>
        )}

        <FeatureSwitch
          label={t('settings.fields.ioThread')}
          description={t('settings.fields.ioThreadHint')}
          checked={draft.ioThread}
          feature={ioThread}
          disabled={locked}
          onChange={(value) => set('ioThread', value)}
        />

        <FeatureSwitch
          label={t('settings.fields.vsock')}
          description={
            vm.vsockCid ? t('settings.fields.vsockAssigned', { cid: vm.vsockCid }) : t('settings.fields.vsockHint')
          }
          checked={draft.vsockEnabled}
          feature={vsock}
          disabled={locked}
          onChange={(value) => set('vsockEnabled', value)}
        />
      </div>
    </SettingsGroup>
  );
}
