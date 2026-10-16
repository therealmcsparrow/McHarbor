// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { formatVmMemory } from './vm-resources';
import {
  IconArrowsExchange,
  IconCopy,
  IconDeviceDesktop,
  IconPlayerPause,
  IconPlayerPlay,
  IconPower,
  IconRotate,
  IconSkull,
  IconSparkles,
  IconTrash,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { GuestAgentDiscButton } from './GuestAgentDiscButton';
import { MakeTemplateButton } from './MakeTemplateButton';
import { VM_PERM } from '../permissions';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import { useHostFeature } from './HostFeatureGate';
import type { LifecycleAction } from '../hooks/useVMs';
import type { VM } from '../types';
import { GuestAgentBadge, StateIndicator } from './VMDetailBadges';

export function VMDetailHeader({
  vm,
  busy,
  diagnosePending,
  onAction,
  onDiagnose,
  onDelete,
  onClone,
  onMigrate,
}: {
  vm: VM;
  busy: boolean;
  diagnosePending: boolean;
  onAction: (action: LifecycleAction) => void;
  onDiagnose: () => void;
  onDelete: () => void;
  onClone: () => void;
  onMigrate: () => void;
}) {
  const { t } = useTranslation('vm');
  const migrateCold = useHostFeature(vm.environmentId, VM_FEATURE.MIGRATE_COLD);
  const stopped = vm.state === 'stopped' || vm.state === 'defined' || vm.state === 'crashed';

  const running = vm.state === 'running';
  const paused = vm.state === 'paused';
  const disabled = busy || !vm.hostAvailable;
  const broken = vm.state === 'crashed' || Boolean(vm.lastError);

  return (
    <>
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconDeviceDesktop className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {vm.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StateIndicator vm={vm} />
            {vm.isTemplate && (
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {t('nav.templates')}
              </Badge>
            )}
            <GuestAgentBadge state={vm.guestAgentState} />
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {t('resources.cpu', { count: vm.vcpus })}
            </span>
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {t('resources.ram', { size: formatVmMemory(vm.memoryMb) })}
            </span>
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {vm.machineType}
            </span>
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {vm.uuid.slice(0, 8)}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex flex-wrap items-center gap-1.5">
          {broken && (
            <HeaderActionButton
              tooltip={t('actions.diagnose')}
              permission={VM_PERM.VIEW}
              onClick={onDiagnose}
              disabled={diagnosePending}
              icon={<IconSparkles className="size-3.5" />}
            />
          )}
          {/* Fixed positions rather than a ternary in one slot. Swapping
            Shutdown for a primary-styled Start under a cursor that is already
            there turns an impatient second click into a power-on. */}
          <HeaderActionButton
            tooltip={t('actions.start')}
            permission={VM_PERM.MANAGE}
            onClick={() => onAction('start')}
            variant="default"
            // A template is deliberately not startable: that immutability is
            // what makes it a template.
            disabled={disabled || vm.isTemplate || running || paused}
            icon={<IconPlayerPlay className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={t('actions.shutdown')}
            permission={VM_PERM.MANAGE}
            onClick={() => onAction('shutdown')}
            disabled={disabled || (!running && !paused)}
            icon={<IconPower className="size-3.5" />}
          />
          <GuestAgentDiscButton vm={vm} appearance="header" disabled={busy} />
          {paused && (
            <HeaderActionButton
              tooltip={t('actions.resume')}
              permission={VM_PERM.MANAGE}
              onClick={() => onAction('resume')}
              variant="default"
              disabled={disabled}
              icon={<IconPlayerPlay className="size-3.5" />}
            />
          )}
          <HeaderActionButton
            tooltip={t('actions.pause')}
            permission={VM_PERM.MANAGE}
            onClick={() => onAction('pause')}
            disabled={disabled || !running}
            icon={<IconPlayerPause className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={t('actions.reset')}
            permission={VM_PERM.MANAGE}
            onClick={() => onAction('reset')}
            disabled={disabled || (!running && !paused)}
            icon={<IconRotate className="size-3.5" />}
          />
          <MakeTemplateButton vm={vm} appearance="header" disabled={busy} />
          <HeaderActionButton
            tooltip={stopped ? t('actions.clone') : `${t('actions.clone')} — ${t('clone.mustBeStopped')}`}
            permission={VM_PERM.MANAGE}
            onClick={onClone}
            disabled={disabled || !stopped}
            icon={<IconCopy className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={migrateCold.supported ? t('actions.migrate') : `${t('actions.migrate')} — ${migrateCold.reason}`}
            permission={VM_PERM.MIGRATE}
            onClick={onMigrate}
            disabled={disabled || vm.isTemplate || !migrateCold.supported}
            icon={<IconArrowsExchange className="size-3.5" />}
          />
          <div className="mx-0.5 h-5 w-px bg-border" />
          <HeaderActionButton
            tooltip={t('actions.stop')}
            permission={VM_PERM.MANAGE}
            onClick={() => onAction('stop')}
            variant="destructive"
            disabled={disabled || (!running && !paused)}
            icon={<IconSkull className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={t('actions.delete')}
            permission={VM_PERM.DELETE}
            onClick={onDelete}
            variant="destructive"
            icon={<IconTrash className="size-3.5" />}
          />
        </div>
      </PageHeadActions>
    </>
  );
}
