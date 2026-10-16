// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import type { TFunction } from 'i18next';
import type { UseMutationResult } from '@tanstack/react-query';
import { IconLink } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { Select } from '@resources/components/ui/Select';
import type { NetworkInfo } from '@core/types/docker';
import type { NetworkConnectPayload } from './NetworkTab';
import {
  emptyNetworkEndpointSettingsDraft,
  NetworkEndpointSettingsFields,
  type NetworkEndpointSettingsDraft,
  splitNetworkAliases,
} from './NetworkEndpointSettingsFields';
import { NetworkTabNetworkCard, type ContainerNetworkEntry } from './NetworkTabNetworkCard';

type NetworkTabNetworksSectionProps = {
  connectMutation: UseMutationResult<unknown, Error, NetworkConnectPayload, unknown>;
  disconnectMutation: UseMutationResult<unknown, Error, string, unknown>;
  editing: boolean;
  readOnly?: boolean;
  allowLiveActionsInEdit?: boolean;
  joinableNetworks: NetworkInfo[];
  networks: ContainerNetworkEntry[];
  selectedNetwork: string;
  setSelectedNetwork: (value: string) => void;
  /** Network mode, edited or shown at the top of the card. */
  modeContent?: ReactNode;
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
  readOnly = false,
  allowLiveActionsInEdit = false,
  joinableNetworks,
  networks,
  selectedNetwork,
  setSelectedNetwork,
  modeContent,
  t,
}: NetworkTabNetworksSectionProps) {
  const [connectDraft, setConnectDraft] = useState(emptyNetworkEndpointSettingsDraft());
  const [editingNetwork, setEditingNetwork] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState(emptyNetworkEndpointSettingsDraft());
  const liveActionsEnabled = !readOnly && (!editing || allowLiveActionsInEdit);

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
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('networkTab.networks')}
      </h3>

      {modeContent}

      {readOnly && <p className="mb-4 text-xs text-muted-foreground">{t('networkTab.readOnlyHint')}</p>}

      {!readOnly && editing && allowLiveActionsInEdit && (
        <p className="mb-4 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          {t('networkTab.liveNetworkActionsNotice')}
        </p>
      )}

      {liveActionsEnabled && joinableNetworks.length > 0 && (
        <div className="mb-4 space-y-3 rounded-md border border-border bg-background/40 p-3">
          <Select
            value={selectedNetwork}
            onChange={setSelectedNetwork}
            options={joinableNetworks.map((network) => ({
              value: network.Name,
              label: network.Name,
            }))}
            placeholder={t('networkTab.selectNetwork')}
            ariaLabel={t('networkTab.selectNetwork')}
            variant="outline"
            className="text-xs"
          />
          <NetworkEndpointSettingsFields
            draft={connectDraft}
            disabled={connectMutation.isPending}
            onChange={(patch) => setConnectDraft((current) => ({ ...current, ...patch }))}
            t={t}
          />
          <Button
            variant="outline"
            size="sm"
            permission={PERM.CONTAINERS_MANAGE}
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
          {networks.map(([networkName, network]) => (
            <NetworkTabNetworkCard
              key={networkName}
              networkName={networkName}
              network={network}
              liveActionsEnabled={liveActionsEnabled}
              isEditing={editingNetwork === networkName}
              editDraft={editDraft}
              connectPending={connectMutation.isPending}
              disconnectPending={disconnectMutation.isPending}
              onStartEdit={() => handleStartEdit(networkName, network)}
              onCancelEdit={() => setEditingNetwork(null)}
              onApplyEdit={() => handleApplyEdit(networkName)}
              onEditDraftChange={(patch) => setEditDraft((current) => ({ ...current, ...patch }))}
              onDisconnect={() => disconnectMutation.mutate(networkName)}
              t={t}
            />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('networkTab.noConnectedNetworks')}</p>
      )}
    </div>
  );
}
