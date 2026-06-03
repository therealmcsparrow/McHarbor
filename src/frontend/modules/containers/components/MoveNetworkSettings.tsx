// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconNetwork } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { NetworkInfo } from '@core/types/docker';
import type { MoveContainerPlan, MoveNetworkConfig } from '../hooks/useContainers';
import { MoveNetworkCard } from './MoveNetworkCard';

type MoveNetworkSettingsProps = {
  networkMode: string;
  networks: MoveNetworkConfig[];
  plan: MoveContainerPlan;
  targetNetworks: NetworkInfo[];
  onNetworkModeChange: (value: string) => void;
  onNetworksChange: (value: MoveNetworkConfig[]) => void;
};

const modeOptions = [
  { value: 'bridge', label: 'bridge' },
  { value: 'host', label: 'host' },
  { value: 'none', label: 'none' },
];
const builtinNetworkModes = new Set(modeOptions.map((option) => option.value));

export function moveNetworkConfigsFromPlan(plan: MoveContainerPlan): MoveNetworkConfig[] {
  return plan.networks.map((network) => ({
    sourceName: network.sourceName || network.name,
    targetName: network.targetName || network.name,
    driver: network.driver || 'bridge',
    internal: network.internal,
    attachable: network.attachable,
    ipam: network.ipam,
    options: network.options,
    labels: network.labels,
    aliases: network.targetAliases ?? network.aliases ?? [],
    ipAddress: network.targetIpAddress ?? '',
    macAddress: network.targetMacAddress ?? '',
  }));
}

export function MoveNetworkSettings({
  networkMode,
  networks,
  plan,
  targetNetworks,
  onNetworkModeChange,
  onNetworksChange,
}: MoveNetworkSettingsProps) {
  const { t } = useTranslation('containers');
  const targetNetworkOptions = targetNetworks
    .map((network) => ({ value: network.Name, label: `${network.Name} (${network.Driver})` }))
    .sort((a, b) => a.label.localeCompare(b.label));
  const networkModeOptions = [
    ...modeOptions,
    ...targetNetworkOptions.filter((option) => !modeOptions.some((mode) => mode.value === option.value)),
  ];
  const modeSelectOptions = networkMode && !networkModeOptions.some((option) => option.value === networkMode)
    ? [...networkModeOptions, { value: networkMode, label: networkMode }]
    : networkModeOptions;

  const targetNetworkPatch = (targetName: string): Partial<MoveNetworkConfig> => {
    const targetNetwork = targetNetworks.find((network) => network.Name === targetName);
    const patch: Partial<MoveNetworkConfig> = { targetName };

    if (targetNetwork) {
      patch.driver = targetNetwork.Driver || 'bridge';
      patch.internal = targetNetwork.Internal;
      patch.attachable = targetNetwork.Attachable;
      patch.ipam = targetNetwork.IPAM;
      patch.options = targetNetwork.Options ?? undefined;
      patch.labels = targetNetwork.Labels ?? undefined;
    }

    return patch;
  };

  const primarySourceName = plan.networks.find((network) => (network.sourceName || network.name) === plan.networkMode)?.sourceName
    || plan.networks.find((network) => network.name === plan.networkMode)?.name
    || networks[0]?.sourceName
    || '';

  const isPrimaryNetwork = (network: MoveNetworkConfig) => network.sourceName === primarySourceName;
  const targetNameFor = (network: MoveNetworkConfig) => isPrimaryNetwork(network)
    ? networkMode
    : network.targetName;

  const handleNetworkModeChange = (value: string) => {
    onNetworkModeChange(value);
    onNetworksChange(networks.map((network) => (
      isPrimaryNetwork(network) ? { ...network, ...targetNetworkPatch(value) } : network
    )));
  };

  const updateNetwork = (sourceName: string, patch: Partial<MoveNetworkConfig>) => {
    onNetworksChange(networks.map((network) => (
      network.sourceName === sourceName ? { ...network, ...patch } : network
    )));
  };

  return (
    <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <IconNetwork className="size-4 text-cyan-400" />
        {t('moveDialog.editNetworkSettings')}
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-1.5">
          <Label>{t('moveDialog.networkMode')}</Label>
          <Select
            value={networkMode}
            onChange={handleNetworkModeChange}
            options={modeSelectOptions}
            ariaLabel={t('moveDialog.networkMode')}
          />
        </div>
      </div>

      <div className="space-y-3">
        {networks.map((network) => {
          const source = plan.networks.find((item) => (item.sourceName || item.name) === network.sourceName);
          const targetName = targetNameFor(network);
          const isBuiltin = builtinNetworkModes.has(targetName);
          const canConfigureEndpoint = targetName !== 'host' && targetName !== 'none';
          const showCreationFields = !!source?.willCreate && !isBuiltin;
          const showEndpointFields = canConfigureEndpoint && (
            !isBuiltin
            || !!source?.ipAddress
            || !!source?.macAddress
            || !!network.ipAddress
            || !!network.macAddress
            || !!network.aliases?.length
          );
          const showNetworkCard = showCreationFields || showEndpointFields || network.sourceName !== targetName;
          if (!showNetworkCard) return null;

          return (
            <MoveNetworkCard
              key={network.sourceName}
              network={network}
              source={source}
              targetName={targetName}
              showCreationFields={showCreationFields}
              showEndpointFields={showEndpointFields}
              updateNetwork={updateNetwork}
              t={t}
            />
          );
        })}
      </div>
    </div>
  );
}
