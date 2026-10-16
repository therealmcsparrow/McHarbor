// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCpu } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useVMs } from '../hooks/useVMs';
import { useWidgetContext } from './mount';

/**
 * Processors and memory given out to running guests, per host.
 *
 * Only running guests are counted. A stopped guest reserves nothing, and
 * including it would overstate the load on every host with a few spare guests
 * defined — which is most of them.
 */
const VCPU_UNIT = 'vCPU';
const GB_UNIT = 'GB';
const ZERO = 0;

export function ResourcesWidget() {
  const { t } = useTranslation('vm');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useVMs();

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const totals = new Map<string, { vcpus: number; memoryMb: number; running: number }>();
  for (const vm of data ?? []) {
    if (vm.isTemplate || vm.state !== 'running') continue;
    const entry = totals.get(vm.environmentId) ?? { vcpus: 0, memoryMb: 0, running: 0 };
    entry.vcpus += vm.vcpus;
    entry.memoryMb += vm.memoryMb;
    entry.running += 1;
    totals.set(vm.environmentId, entry);
  }

  if (totals.size === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconCpu className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">
          {t('list.filter.running')}: {ZERO}
        </p>
      </div>
    );
  }

  const hosts = [...totals.entries()].sort((a, b) => b[1].vcpus - a[1].vcpus);
  const grandVCPUs = hosts.reduce((sum, [, entry]) => sum + entry.vcpus, 0);
  const grandMemory = hosts.reduce((sum, [, entry]) => sum + entry.memoryMb, 0);

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-kvm')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-3 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-3">
        <span className="text-2xl font-semibold text-foreground">{grandVCPUs}</span>
        <span className="text-sm text-muted-foreground">
          {VCPU_UNIT} · {Math.round(grandMemory / 1024)} {GB_UNIT}
        </span>
      </div>

      <ul className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {hosts.map(([host, entry]) => (
          <li key={host} className="flex items-center justify-between gap-2 text-xs">
            <span className="min-w-0 truncate text-foreground">{host}</span>
            <span className="shrink-0 text-muted-foreground">
              {entry.vcpus} {VCPU_UNIT} · {Math.round(entry.memoryMb / 1024)} {GB_UNIT} · {entry.running}
            </span>
          </li>
        ))}
      </ul>
    </Button>
  );
}
