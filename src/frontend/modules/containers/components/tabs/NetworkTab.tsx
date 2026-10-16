// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import { useContainers } from '@resources/hooks/useContainers';
import { useImage } from '@resources/hooks/useImages';
import type { EditFormData, PortMappingEntry } from '../../types/edit-form';
import { exposedPortRows } from '../../types/exposed-ports';
import { useKnownAgentNetworks, useNetworkEditHandlers } from '../../hooks/useNetworkTabHooks';
import { NetworkTabLayout } from './NetworkTabLayout';

export type NetworkConnectPayload = {
  network: string;
  aliases?: string[];
  ipv4Address?: string;
  ipv6Address?: string;
  macAddress?: string;
  reconnect?: boolean;
};

const NO_PORT_MAPPINGS: PortMappingEntry[] = [];

type NetworkTabProps = {
  container: ContainerInspect;
  editing?: boolean;
  allowLiveNetworkActionsInEdit?: boolean;
  /** The detail page shows networks read-only; connecting, disconnecting
      and endpoint edits happen in the configuration editor. */
  readOnly?: boolean;
  /** "editor" puts Networks in its own right-hand column, as the configuration editor wants. */
  layout?: 'grid' | 'editor';
  editData?: EditFormData | null;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function NetworkTab({
  container,
  editing = false,
  allowLiveNetworkActionsInEdit = false,
  readOnly = false,
  layout = 'grid',
  editData,
  onFieldChange,
}: NetworkTabProps) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const currentEnvironment = useEnvironmentStore((s) => s.environments.find((env) => env.id === s.currentId));
  const isAgentEnvironment = currentEnvironment?.connectionType === 'agent';
  const envQuery = envId ? `?env=${envId}` : '';
  const [selectedNetwork, setSelectedNetwork] = useState('');

  const connectedNetworkNames = useMemo(
    () => new Set(Object.keys(container.NetworkSettings?.Networks ?? {})),
    [container.NetworkSettings?.Networks],
  );
  const { data: containers = [] } = useContainers(true);

  const { data: availableNetworks } = useQuery<NetworkInfo[]>({
    queryKey: ['networks', envId],
    queryFn: () => api.get<NetworkInfo[]>(`/networks${envQuery}`).then((r) => r.data ?? []),
    enabled: !isAgentEnvironment,
  });

  const connectMutation = useMutation({
    mutationFn: (payload: NetworkConnectPayload) =>
      api.post(`/containers/${container.Id}/network/connect${envQuery}`, payload).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      setSelectedNetwork('');
    },
  });

  const disconnectMutation = useMutation({
    mutationFn: (network: string) =>
      api
        .post(`/containers/${container.Id}/network/disconnect${envQuery}`, {
          network,
        })
        .then(assertSuccess),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['container'] }),
  });

  // Port mappings
  const publishedPortMappings = container.NetworkSettings?.Ports
    ? Object.entries(container.NetworkSettings.Ports).flatMap(([containerBinding, bindings]) => {
        const [containerPort = '', protocol = 'tcp'] = containerBinding.split('/');
        return (bindings ?? []).map((binding) => ({
          hostIp: binding.HostIp || '0.0.0.0',
          hostPort: binding.HostPort,
          containerPort,
          protocol,
        }));
      })
    : [];

  const networks = Object.entries(container.NetworkSettings?.Networks ?? {});
  const exposedPorts = container.Config?.ExposedPorts ? Object.keys(container.Config.ExposedPorts) : [];
  const portMappings = editData?.portMappings ?? NO_PORT_MAPPINGS;
  // The image's own EXPOSE ports come back on every recreate, so the editor locks them.
  const imageQuery = useImage(editing ? (container.Image ?? '') : '');
  const imageExposed = imageQuery.data?.Config?.ExposedPorts;
  const imagePorts = useMemo(() => new Set(Object.keys(imageExposed ?? {})), [imageExposed]);
  const exposedRows = useMemo(
    () => exposedPortRows(editData?.exposedPorts ?? [], portMappings, imagePorts),
    [editData?.exposedPorts, portMappings, imagePorts],
  );

  const dns = editing ? (editData?.dns ?? []) : (container.HostConfig?.Dns ?? []);
  const dnsSearch = editing ? (editData?.dnsSearch ?? []) : (container.HostConfig?.DnsSearch ?? []);
  const dnsOptions = editing ? (editData?.dnsOptions ?? []) : (container.HostConfig?.DnsOptions ?? []);
  const extraHosts = useMemo(
    () => (editing ? (editData?.extraHosts ?? []) : (container.HostConfig?.ExtraHosts ?? [])),
    [container.HostConfig?.ExtraHosts, editData?.extraHosts, editing],
  );
  const portMappingKeys = useStableListKeys(
    portMappings,
    (mapping) => `${mapping.hostIp}\u0000${mapping.hostPort}\u0000${mapping.containerPort}\u0000${mapping.protocol}`,
  );
  const extraHostKeys = useStableListKeys(extraHosts, (host) => host);

  const {
    handlePortMappingChange,
    handleAddPortMapping,
    handleRemovePortMapping,
    handleRemoveExposedPort,
    handleExtraHostChange,
    handleAddExtraHost,
    handleRemoveExtraHost,
  } = useNetworkEditHandlers(editData, extraHosts, onFieldChange, imagePorts);

  const knownAgentNetworks = useKnownAgentNetworks(isAgentEnvironment, connectedNetworkNames, containers);
  const resolvedAvailableNetworks = isAgentEnvironment ? knownAgentNetworks : (availableNetworks ?? []);
  const joinableNetworks = resolvedAvailableNetworks.filter((n: NetworkInfo) => !connectedNetworkNames.has(n.Name));

  const portsProps = {
    containerId: container.Id,
    allowConnectionTest: ['socket'].includes(currentEnvironment?.connectionType ?? ''),
    editing,
    exposedPorts,
    exposedRows,
    onRemoveExposedPort: handleRemoveExposedPort,
    portMappingKeys,
    portMappings,
    publishedPortMappings,
    onAddPortMapping: handleAddPortMapping,
    onPortMappingChange: handlePortMappingChange,
    onRemovePortMapping: handleRemovePortMapping,
  };
  const configProps = {
    connectMutation,
    container,
    disconnectMutation,
    dns,
    dnsOptions,
    dnsSearch,
    editing,
    editData,
    allowLiveNetworkActionsInEdit,
    readOnly,
    availableNetworks: resolvedAvailableNetworks,
    extraHostKeys,
    extraHosts,
    joinableNetworks,
    networks,
    onAddExtraHost: handleAddExtraHost,
    onExtraHostChange: handleExtraHostChange,
    onFieldChange,
    onRemoveExtraHost: handleRemoveExtraHost,
    selectedNetwork,
    setSelectedNetwork,
  };

  return <NetworkTabLayout layout={layout} ports={portsProps} config={configProps} />;
}
