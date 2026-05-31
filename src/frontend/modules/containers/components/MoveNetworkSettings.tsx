// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconNetwork } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Badge } from '@resources/components/ui/Badge';
import { DRIVER_OPTIONS } from '@resources/constants/network-drivers';
import type { NetworkInfo } from '@core/types/docker';
import type { MoveContainerPlan, MoveNetworkConfig } from '../hooks/useContainers';
import { csv, splitCsv, updateIpam } from './move-network-settings-utils';

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

  const updateNetwork = (sourceName: string, patch: Partial<MoveNetworkConfig>) => {
    const currentNetwork = networks.find((network) => network.sourceName === sourceName);
    const nextNetworks = networks.map((network) => (
      network.sourceName === sourceName ? { ...network, ...patch } : network
    ));
    if (patch.targetName && (networkMode === sourceName || networkMode === currentNetwork?.targetName)) {
      onNetworkModeChange(patch.targetName);
    }
    onNetworksChange(nextNetworks);
  };

  const applyTargetNetwork = (sourceName: string, targetName: string) => {
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

    updateNetwork(sourceName, patch);
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
            onChange={onNetworkModeChange}
            options={modeSelectOptions}
            ariaLabel={t('moveDialog.networkMode')}
          />
        </div>
      </div>

      <div className="space-y-3">
        {networks.map((network) => {
          const source = plan.networks.find((item) => (item.sourceName || item.name) === network.sourceName);
          const ipam = network.ipam?.Config?.[0] ?? {};
          return (
            <div key={network.sourceName} className="space-y-3 rounded-md border border-border bg-background/50 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="text-sm font-medium">
                  {source?.name ?? network.sourceName}
                  <span className="ml-2 text-xs text-muted-foreground">{t('moveDialog.to')}</span>
                  <span className="ml-2 text-sm text-foreground">{network.targetName}</span>
                </div>
                <Badge variant={source?.willCreate ? 'warning' : 'secondary'}>
                  {source?.willCreate ? t('moveDialog.create') : t('moveDialog.keep')}
                </Badge>
              </div>

              <div className="grid gap-3 md:grid-cols-3">
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.targetNetwork')}</Label>
                  <Select
                    value={network.targetName}
                    onChange={(value) => applyTargetNetwork(network.sourceName, value)}
                    options={
                      network.targetName && !targetNetworkOptions.some((option) => option.value === network.targetName)
                        ? [{ value: network.targetName, label: network.targetName }, ...targetNetworkOptions]
                        : targetNetworkOptions
                    }
                    placeholder={t('networkTab.selectNetwork')}
                    ariaLabel={t('moveDialog.targetNetwork')}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.driver')}</Label>
                  <Select value={network.driver ?? 'bridge'} onChange={(value) => updateNetwork(network.sourceName, { driver: value })} options={[...DRIVER_OPTIONS]} />
                </div>
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.aliases')}</Label>
                  <Input value={csv(network.aliases)} onChange={(event) => updateNetwork(network.sourceName, { aliases: splitCsv(event.target.value) })} />
                </div>
              </div>

              <div className="grid gap-3 md:grid-cols-3">
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.subnet')}</Label>
                  <Input value={ipam.Subnet ?? ''} onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'Subnet', event.target.value))} placeholder="172.20.0.0/16" />
                </div>
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.gateway')}</Label>
                  <Input value={ipam.Gateway ?? ''} onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'Gateway', event.target.value))} placeholder="172.20.0.1" />
                </div>
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.ipRange')}</Label>
                  <Input value={ipam.IPRange ?? ''} onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'IPRange', event.target.value))} placeholder="172.20.5.0/24" />
                </div>
              </div>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.targetIp')}</Label>
                  <Input value={network.ipAddress ?? ''} onChange={(event) => updateNetwork(network.sourceName, { ipAddress: event.target.value })} placeholder={source?.ipAddress} />
                </div>
                <div className="space-y-1.5">
                  <Label>{t('moveDialog.targetMac')}</Label>
                  <Input value={network.macAddress ?? ''} onChange={(event) => updateNetwork(network.sourceName, { macAddress: event.target.value })} placeholder={source?.macAddress} />
                </div>
              </div>

              <div className="flex flex-wrap gap-4">
                <Label className="flex items-center gap-2 text-sm">
                  <Switch checked={!!network.internal} onCheckedChange={(checked) => updateNetwork(network.sourceName, { internal: checked })} />
                  {t('moveDialog.internal')}
                </Label>
                <Label className="flex items-center gap-2 text-sm">
                  <Switch checked={!!network.attachable} onCheckedChange={(checked) => updateNetwork(network.sourceName, { attachable: checked })} />
                  {t('moveDialog.attachable')}
                </Label>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
