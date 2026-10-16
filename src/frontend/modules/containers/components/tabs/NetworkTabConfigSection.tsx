// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Fragment, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Badge } from '@resources/components/ui/Badge';
import { Select } from '@resources/components/ui/Select';
import type { UseMutationResult } from '@tanstack/react-query';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';
import type { NetworkConnectPayload } from './NetworkTab';
import { NetworkTabNetworksSection } from './NetworkTabNetworksSection';
import { NetworkDnsCard } from './NetworkDnsCard';
import { NetworkExtraHostsCard } from './NetworkExtraHostsCard';

export type ConfigPart = 'dns' | 'extraHosts' | 'networks';

const DEFAULT_PARTS: ConfigPart[] = ['dns', 'extraHosts', 'networks'];

type NetworkTabConfigSectionProps = {
  availableNetworks: NetworkInfo[];
  connectMutation: UseMutationResult<unknown, Error, NetworkConnectPayload, unknown>;
  container: ContainerInspect;
  disconnectMutation: UseMutationResult<unknown, Error, string, unknown>;
  dns: string[];
  dnsOptions: string[];
  dnsSearch: string[];
  editing: boolean;
  editData?: EditFormData | null;
  allowLiveNetworkActionsInEdit: boolean;
  readOnly?: boolean;
  /** The cards to render, in this order; the configuration editor splits them across two columns. */
  parts?: ConfigPart[];
  extraHostKeys: string[];
  extraHosts: string[];
  joinableNetworks: NetworkInfo[];
  networks: [string, NonNullable<ContainerInspect['NetworkSettings']>['Networks'][string]][];
  onAddExtraHost: () => void;
  onExtraHostChange: (index: number, value: string) => void;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
  onRemoveExtraHost: (index: number) => void;
  selectedNetwork: string;
  setSelectedNetwork: (value: string) => void;
};

const builtinNetworkModes = new Set(['bridge', 'host', 'none']);

export function NetworkTabConfigSection({
  availableNetworks,
  connectMutation,
  container,
  disconnectMutation,
  dns,
  dnsOptions,
  dnsSearch,
  editing,
  editData,
  allowLiveNetworkActionsInEdit,
  readOnly = false,
  parts = DEFAULT_PARTS,
  extraHostKeys,
  extraHosts,
  joinableNetworks,
  networks,
  onAddExtraHost,
  onExtraHostChange,
  onFieldChange,
  onRemoveExtraHost,
  selectedNetwork,
  setSelectedNetwork,
}: NetworkTabConfigSectionProps) {
  const { t } = useTranslation('containers');
  const selectedNetworkMode = editData?.networkMode ?? '';
  const networkModeOptions = [
    ...Array.from(builtinNetworkModes).map((mode) => ({
      value: mode,
      label: mode,
    })),
    ...availableNetworks
      .filter((network) => !builtinNetworkModes.has(network.Name))
      .map((network) => ({
        value: network.Name,
        label: `${network.Name} (${network.Driver})`,
      }))
      .sort((a, b) => a.label.localeCompare(b.label)),
  ];
  const shouldAddCurrentNetworkMode =
    !!selectedNetworkMode && !networkModeOptions.some((option) => option.value === selectedNetworkMode);

  // Network mode is part of how the container joins networks, so it heads the Networks card.
  const modeContent = editing ? (
    <div className="mb-4">
      <label className="text-xs font-medium text-muted-foreground">{t('networkTab.networkMode')}</label>
      <Select
        value={selectedNetworkMode}
        onChange={(next) => onFieldChange?.('networkMode', next)}
        options={[
          ...networkModeOptions,
          ...(shouldAddCurrentNetworkMode ? [{ value: selectedNetworkMode, label: selectedNetworkMode }] : []),
        ]}
        ariaLabel={t('networkTab.networkMode')}
        variant="outline"
        className="mt-1 text-xs"
      />
    </div>
  ) : (
    <div className="mb-4">
      <InfoRow label={t('networkTab.hostname')}>{container.Config?.Hostname ?? '-'}</InfoRow>
      <InfoRow label={t('networkTab.networkMode')}>
        <Badge variant="secondary">{container.HostConfig?.NetworkMode ?? '-'}</Badge>
      </InfoRow>
    </div>
  );

  const cards: Record<ConfigPart, ReactNode> = {
    dns: (
      <NetworkDnsCard
        dns={dns}
        dnsOptions={dnsOptions}
        dnsSearch={dnsSearch}
        editing={editing}
        onFieldChange={onFieldChange}
      />
    ),
    extraHosts: (
      <NetworkExtraHostsCard
        editing={editing}
        extraHostKeys={extraHostKeys}
        extraHosts={extraHosts}
        onAddExtraHost={onAddExtraHost}
        onExtraHostChange={onExtraHostChange}
        onRemoveExtraHost={onRemoveExtraHost}
      />
    ),
    networks: (
      <NetworkTabNetworksSection
        connectMutation={connectMutation}
        disconnectMutation={disconnectMutation}
        editing={editing}
        readOnly={readOnly}
        allowLiveActionsInEdit={allowLiveNetworkActionsInEdit}
        joinableNetworks={joinableNetworks}
        networks={networks}
        selectedNetwork={selectedNetwork}
        setSelectedNetwork={setSelectedNetwork}
        modeContent={modeContent}
        t={t}
      />
    ),
  };

  return (
    <>
      {parts.map((part) => (
        <Fragment key={part}>{cards[part]}</Fragment>
      ))}
    </>
  );
}
