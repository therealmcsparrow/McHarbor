// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { VM_PERM } from '../permissions';
import { useResizeMemory, useSetVCPUs, type ResizeResult } from '../hooks/useResize';
import { useHostFeature } from './HostFeatureGate';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import type { VM } from '../types';

/**
 * Memory and processor changes on a running guest.
 *
 * Both are requests the guest may decline — a memory request is rounded to the
 * host's block size, and an unplug regularly fails outright on Windows — so
 * the panel reports what the guest ended up with rather than what was asked
 * for. A refusal is a visible state here, not a silent no-op.
 */
export function LiveResizePanel({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const memoryFeature = useHostFeature(vm.environmentId, VM_FEATURE.VIRTIO_MEM);
  const cpuFeature = useHostFeature(vm.environmentId, VM_FEATURE.CPU_HOTPLUG);

  const resizeMemory = useResizeMemory(vm.id);
  const setVCPUs = useSetVCPUs(vm.id);

  const [memoryMb, setMemoryMb] = useState(String(vm.memoryCurrentMb || vm.memoryMb));
  const [vcpus, setVcpus] = useState(String(vm.vcpus));

  const running = vm.state === 'running';
  const memoryEnabled = running && Boolean(vm.virtioMem) && memoryFeature.supported;
  const cpuEnabled = running && (vm.vcpusMax ?? 0) > vm.vcpus && cpuFeature.supported;

  return (
    <section className="space-y-4">
      <div>
        <h3 className="font-medium text-foreground">{t('resize.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('resize.description')}</p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <ResizeRow
          label={t('resize.memory')}
          hint={
            vm.memoryCurrentMb && vm.memoryCurrentMb !== vm.memoryMb
              ? t('resize.memoryNow', {
                  boot: vm.memoryMb,
                  now: vm.memoryCurrentMb,
                })
              : t('resize.memoryBoot', { boot: vm.memoryMb })
          }
          value={memoryMb}
          onChange={setMemoryMb}
          unit={t('resize.mb')}
          enabled={memoryEnabled}
          disabledReason={
            !running ? t('resize.notRunning') : !vm.virtioMem ? t('resize.noVirtioMem') : memoryFeature.reason
          }
          pending={resizeMemory.isPending}
          result={resizeMemory.data}
          onApply={() => resizeMemory.mutate(Number(memoryMb))}
          formatResult={(result) =>
            t('resize.memoryResult', {
              requested: result.requestedMemoryMb ?? 0,
              actual: result.actualMemoryMb ?? 0,
            })
          }
        />

        <ResizeRow
          label={t('resize.vcpus')}
          hint={t('resize.vcpusCeiling', { max: vm.vcpusMax ?? vm.vcpus })}
          value={vcpus}
          onChange={setVcpus}
          unit={t('resize.processors')}
          enabled={cpuEnabled}
          disabledReason={
            !running
              ? t('resize.notRunning')
              : (vm.vcpusMax ?? 0) <= vm.vcpus
                ? t('resize.noHeadroom')
                : cpuFeature.reason
          }
          pending={setVCPUs.isPending}
          result={setVCPUs.data}
          onApply={() => setVCPUs.mutate(Number(vcpus))}
          formatResult={(result) =>
            t('resize.vcpusResult', {
              requested: result.requestedVcpus ?? 0,
              actual: result.actualVcpus ?? 0,
            })
          }
        />
      </div>
    </section>
  );
}

function ResizeRow({
  label,
  hint,
  value,
  onChange,
  unit,
  enabled,
  disabledReason,
  pending,
  result,
  onApply,
  formatResult,
}: {
  label: string;
  hint: string;
  value: string;
  onChange: (value: string) => void;
  unit: string;
  enabled: boolean;
  disabledReason: string;
  pending: boolean;
  result?: ResizeResult;
  onApply: () => void;
  formatResult: (result: ResizeResult) => string;
}) {
  const { t } = useTranslation('vm');

  return (
    <div className="space-y-2 rounded-lg border border-border bg-card p-4">
      <div className="text-sm font-medium text-foreground">{label}</div>
      <div className="text-sm text-muted-foreground">{hint}</div>
      <div className="flex items-center gap-2">
        <NumberInput
          nullable
          value={value.trim() === '' ? null : Number(value)}
          disabled={!enabled}
          onChange={(next) => onChange(next === null ? '' : String(next))}
          suffix={unit}
          className="max-w-[260px]"
          aria-label={label}
        />
      </div>

      {result && (
        <p
          className={
            result.complied ? 'text-sm text-muted-foreground' : 'flex items-start gap-1.5 text-sm text-destructive'
          }
        >
          {!result.complied && <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />}
          <span>
            {formatResult(result)}
            {!result.complied && ` ${t('resize.declined')}`}
          </span>
        </p>
      )}

      <div className="flex justify-end">
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button size="sm" permission={VM_PERM.MANAGE} disabled={!enabled || pending} onClick={onApply}>
                {t('resize.apply')}
              </Button>
            </span>
          </TooltipTrigger>
          {!enabled && <TooltipContent className="max-w-[320px]">{disabledReason}</TooltipContent>}
        </Tooltip>
      </div>
    </div>
  );
}
