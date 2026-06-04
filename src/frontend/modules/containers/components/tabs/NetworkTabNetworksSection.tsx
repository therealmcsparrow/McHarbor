// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import type { TFunction } from 'i18next';
import type { UseMutationResult } from '@tanstack/react-query';
import { IconCheck, IconLink, IconPencil, IconUnlink, IconX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import type { NetworkConnectPayload } from './NetworkTab';
import {
  emptyNetworkEndpointSettingsDraft,
  NetworkEndpointSettingsFields,
  type NetworkEndpointSettingsDraft,
  splitNetworkAliases,
} from './NetworkEndpointSettingsFields';

type ContainerNetworkEntry = [
  string,
  NonNullable<ContainerInspect['NetworkSettings']>['Networks'][string],
];

type NetworkTabNetworksSectionProps = {
  connectMutation: UseMutationResult<unknown, Error, NetworkConnectPayload, unknown>;
  disconnectMutation: UseMutationResult<unknown, Error, string, unknown>;
  editing: boolean;
  joinableNetworks: NetworkInfo[];
  networks: ContainerNetworkEntry[];
  selectedNetwork: string;
  setSelectedNetwork: (value: string) => void;
  t: TFunction<'containers'>;
};

function draftFromNetwork(network: ContainerNetworkEntry[1]): NetworkEndpointSettingsDraft {
  return {
    aliases: (network?.Aliases ?? []).join(', '),
    ipv4Address: network?.IPAddress ?? '',
    ipv6Address: network?.GlobalIPv6Address ?? '',
    macAddress: network?.MacAddress ?? '',
  };
}

function payloadFromDraft(
  network: string,
  draft: NetworkEndpointSettingsDraft,
  reconnect = false,
): NetworkConnectPayload {
  return {
    network,
    aliases: splitNetworkAliases(draft.aliases),
    ipv4Address: draft.ipv4Address.trim(),
    ipv6Address: draft.ipv6Address.trim(),
    macAddress: draft.macAddress.trim(),
    reconnect,
  };
}

export function NetworkTabNetworksSection({
  connectMutation,
  disconnectMutation,
  editing,
  joinableNetworks,
  networks,
  selectedNetwork,
  setSelectedNetwork,
  t,
}: NetworkTabNetworksSectionProps) {
  const [connectDraft, setConnectDraft] = useState(emptyNetworkEndpointSettingsDraft());
  const [editingNetwork, setEditingNetwork] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState(emptyNetworkEndpointSettingsDraft());

  const handleConnect = () => {
    if (!selectedNetwork) return;
    connectMutation.mutate(payloadFromDraft(selectedNetwork, connectDraft), {
      onSuccess: () => setConnectDraft(emptyNetworkEndpointSettingsDraft()),
    });
  };

  const handleStartEdit = (networkName: string, network: ContainerNetworkEntry[1]) => {
    setEditingNetwork(networkName);
    setEditDraft(draftFromNetwork(network));
  };

  const handleApplyEdit = (networkName: string) => {
    connectMutation.mutate(payloadFromDraft(networkName, editDraft, true), {
      onSuccess: () => setEditingNetwork(null),
    });
  };

  return (
    <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t('networkTab.networks')}
      </h3>

      {!editing && joinableNetworks.length > 0 && (
        <div className="mb-4 space-y-3 rounded-md border border-border bg-background/40 p-3">
          <select
            value={selectedNetwork}
            onChange={(event) => setSelectedNetwork(event.target.value)}
            className="w-full rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
          >
            <option value="">{t('networkTab.selectNetwork')}</option>
            {joinableNetworks.map((network) => (
              <option key={network.Id} value={network.Name}>{network.Name}</option>
            ))}
          </select>
          <NetworkEndpointSettingsFields
            draft={connectDraft}
            disabled={connectMutation.isPending}
            onChange={(patch) => setConnectDraft((current) => ({ ...current, ...patch }))}
            t={t}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleConnect}
            disabled={!selectedNetwork || connectMutation.isPending}
          >
            <IconLink className="mr-1 size-3.5" />
            {t('actions.connect')}
          </Button>
        </div>
      )}

      {networks.length > 0 ? (
        <div className="space-y-4">
          {networks.map(([networkName, network]) => {
            const isEditingNetwork = editingNetwork === networkName;
            return (
              <div key={networkName} className="rounded-md border border-border p-4">
                <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <Badge variant="secondary">{networkName}</Badge>
                  {!editing && (
                    <div className="flex gap-2">
                      {isEditingNetwork ? (
                        <>
                          <Button variant="outline" size="sm" onClick={() => handleApplyEdit(networkName)} disabled={connectMutation.isPending}>
                            <IconCheck className="mr-1 size-3.5" />
                            {t('networkTab.applyNetworkSettings')}
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setEditingNetwork(null)}>
                            <IconX className="mr-1 size-3.5" />
                            {t('networkTab.cancelNetworkEdit')}
                          </Button>
                        </>
                      ) : (
                        <Button variant="ghost" size="sm" onClick={() => handleStartEdit(networkName, network)}>
                          <IconPencil className="mr-1 size-3.5" />
                          {t('networkTab.editNetworkSettings')}
                        </Button>
                      )}
                      <Button variant="ghost" size="sm" onClick={() => disconnectMutation.mutate(networkName)} disabled={disconnectMutation.isPending} className="text-red-500 hover:text-red-600">
                        <IconUnlink className="mr-1 size-3.5" />
                        {t('actions.disconnect')}
                      </Button>
                    </div>
                  )}
                </div>

                {isEditingNetwork ? (
                  <div className="space-y-3">
                    <NetworkEndpointSettingsFields
                      draft={editDraft}
                      disabled={connectMutation.isPending}
                      onChange={(patch) => setEditDraft((current) => ({ ...current, ...patch }))}
                      t={t}
                    />
                    <p className="text-xs text-muted-foreground">{t('networkTab.reconnectNotice')}</p>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 gap-x-8 sm:grid-cols-2">
                    <InfoRow label={t('networkTab.ipAddress')}>{network?.IPAddress || '-'}</InfoRow>
                    <InfoRow label={t('networkTab.gateway')}>{network?.Gateway || '-'}</InfoRow>
                    <InfoRow label={t('networkTab.macAddress')}>{network?.MacAddress || '-'}</InfoRow>
                    <InfoRow label={t('networkTab.prefixLength')}>{network?.IPPrefixLen ?? '-'}</InfoRow>
                    {(network?.Aliases?.length ?? 0) > 0 && (
                      <InfoRow label={t('networkTab.aliases')}>{network.Aliases?.join(', ')}</InfoRow>
                    )}
                    {network?.GlobalIPv6Address && (
                      <>
                        <InfoRow label={t('networkTab.ipv6Address')}>{network.GlobalIPv6Address}</InfoRow>
                        <InfoRow label={t('networkTab.ipv6Gateway')}>{network.IPv6Gateway || '-'}</InfoRow>
                      </>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('networkTab.noConnectedNetworks')}</p>
      )}
    </div>
  );
}
