// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useHostEnvironmentNames } from '../hooks/useHostEnvironments';
import { InstallGuestAgentDialog } from './InstallGuestAgentDialog';
import { Section } from './DetailSection';
import { MachineTypeWarning } from './HostFeatureGate';
import { VMMetricsSection } from './VMMetricsSection';
import { formatBytes, formatRelative } from '../format';
import type { Stats, VM } from '../types';

function GuestSection({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const agent = vm.guestAgentState || 'unknown';
  const ips = vm.guestReportedIps ?? [];
  const [installing, setInstalling] = useState(false);

  return (
    <Section title={t('detail.overview.guest')}>
      <InfoRow label={t('detail.overview.guestOs')}>{vm.guestOsVariant || vm.guestOsFamily || '—'}</InfoRow>
      <InfoRow label={t('detail.overview.guest')}>
        <span className="flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <span>{t(`guestAgent.${agent}`)}</span>
            </TooltipTrigger>
            <TooltipContent>{t('guestAgent.absentHint')}</TooltipContent>
          </Tooltip>
          <Button variant="link" size="sm" className="h-auto p-0" onClick={() => setInstalling(true)}>
            {t('guestAgent.install.action')}
          </Button>
        </span>
      </InfoRow>
      <InfoRow label={t('address.reported')}>
        {ips.length > 0 ? <span className="font-mono text-xs">{ips.join(', ')}</span> : '—'}
      </InfoRow>
      {installing && <InstallGuestAgentDialog vm={vm} onClose={() => setInstalling(false)} />}
    </Section>
  );
}

export function VMOverviewTab({ vm, stats, statsUnavailable }: { vm: VM; stats?: Stats; statsUnavailable?: boolean }) {
  const { t } = useTranslation('vm');
  const hostNames = useHostEnvironmentNames();

  const diskBytes = (vm.disks ?? []).reduce((sum, disk) => sum + disk.sizeBytes, 0);

  return (
    <div className="space-y-6">
      <VMMetricsSection vm={vm} stats={stats} unavailable={statsUnavailable} />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Section title={t('detail.overview.general')}>
          <InfoRow label={t('detail.overview.name')}>{vm.name}</InfoRow>
          <InfoRow label={t('detail.overview.description')}>
            {vm.description || t('detail.overview.noDescription')}
          </InfoRow>
          <InfoRow label={t('detail.overview.host')}>{hostNames.get(vm.environmentId) || vm.environmentId}</InfoRow>
          <InfoRow label={t('detail.overview.state')}>{t(`state.${vm.state}`)}</InfoRow>
          <InfoRow label={t('detail.overview.autostart')}>
            {vm.autostart ? t('labels.yes', { ns: 'common' }) : t('labels.no', { ns: 'common' })}
          </InfoRow>
          <InfoRow label={t('detail.overview.created')}>{formatRelative(vm.createdAt)}</InfoRow>
          <InfoRow label={t('detail.overview.started')}>
            {vm.lastStartedAt ? formatRelative(vm.lastStartedAt) : t('detail.overview.neverStarted')}
          </InfoRow>
        </Section>

        <Section title={t('detail.overview.resources')}>
          <InfoRow label={t('detail.overview.processors')}>
            {vm.vcpus} × <span className="font-mono text-xs">{vm.cpuModel}</span>
          </InfoRow>
          <InfoRow label={t('detail.overview.topology')}>
            {t('detail.overview.topologyValue', {
              sockets: vm.cpuSockets,
              cores: vm.cpuCores,
              threads: vm.cpuThreads,
            })}
          </InfoRow>
          <InfoRow label={t('detail.overview.memory')}>{`${vm.memoryMb} MB`}</InfoRow>
          {vm.memoryMaxMb ? <InfoRow label={t('detail.overview.memoryMax')}>{`${vm.memoryMaxMb} MB`}</InfoRow> : null}
          <InfoRow label={t('detail.overview.disk')}>{formatBytes(diskBytes)}</InfoRow>
          <InfoRow label={t('hardware.disks')}>{(vm.disks ?? []).length}</InfoRow>
          <InfoRow label={t('hardware.nics')}>{(vm.nics ?? []).length}</InfoRow>
          <InfoRow label={t('hardware.devices')}>{(vm.devices ?? []).length}</InfoRow>
        </Section>

        <GuestSection vm={vm} />

        <Section title={t('detail.overview.platform')}>
          <InfoRow label={t('detail.overview.firmware')}>{vm.firmware === 'uefi' ? 'UEFI' : 'BIOS'}</InfoRow>
          <InfoRow label={t('detail.overview.secureBoot')}>
            {vm.secureBoot ? t('labels.yes', { ns: 'common' }) : t('labels.no', { ns: 'common' })}
          </InfoRow>
          <InfoRow label={t('detail.overview.tpm')}>
            {vm.tpmEnabled ? t('labels.yes', { ns: 'common' }) : t('labels.no', { ns: 'common' })}
          </InfoRow>
          <InfoRow label={t('detail.overview.machineType')}>
            <span className="inline-flex items-center gap-1.5">
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="font-mono text-xs">{vm.machineType}</span>
                </TooltipTrigger>
                <TooltipContent>{t('detail.overview.machineTypeHint')}</TooltipContent>
              </Tooltip>
              <MachineTypeWarning environmentId={vm.environmentId} machineType={vm.machineType} />
            </span>
          </InfoRow>
          <InfoRow label={t('detail.overview.display')}>{vm.displayType || '—'}</InfoRow>
          <InfoRow label={t('detail.overview.uuid')}>
            <span className="font-mono text-xs">{vm.uuid}</span>
          </InfoRow>
          {vm.unitName ? (
            <InfoRow label={t('detail.overview.unitName')}>
              <span className="font-mono text-xs">{vm.unitName}</span>
            </InfoRow>
          ) : null}
        </Section>
      </div>

      {vm.lastError && (
        <Section title={t('detail.overview.lastError')}>
          <p className="text-sm text-danger">{vm.lastError}</p>
        </Section>
      )}
    </div>
  );
}
