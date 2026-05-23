// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import type { EditFormData, PortMappingEntry } from '../../types/edit-form';
import { NetworkTabConfigSection } from './NetworkTabConfigSection';
import { NetworkTabPortsSection } from './NetworkTabPortsSection';

type NetworkTabProps = {
  container: ContainerInspect;
  editing?: boolean;
  editData?: EditFormData | null;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function NetworkTab({ container, editing = false, editData, onFieldChange }: NetworkTabProps) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const envQuery = envId ? `?env=${envId}` : '';
  const [selectedNetwork, setSelectedNetwork] = useState('');

  const connectedNetworkNames = new Set(Object.keys(container.NetworkSettings?.Networks ?? {}));

  const { data: availableNetworks } = useQuery<NetworkInfo[]>({
    queryKey: ['networks', envId],
    queryFn: () => api.get<NetworkInfo[]>(`/networks${envQuery}`).then((r) => r.data ?? []),
    enabled: !editing,
  });

  const connectMutation = useMutation({
    mutationFn: (network: string) =>
      api.post(`/containers/${container.Id}/network/connect${envQuery}`, { network }).then(assertSuccess),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container'] });
      setSelectedNetwork('');
    },
  });

  const disconnectMutation = useMutation({
    mutationFn: (network: string) =>
      api.post(`/containers/${container.Id}/network/disconnect${envQuery}`, { network }).then(assertSuccess),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['container'] }),
  });

  // Port mappings
  const ports = container.NetworkSettings?.Ports
    ? Object.entries(container.NetworkSettings.Ports)
        .filter(([, bindings]) => bindings && bindings.length > 0)
        .map(([port, bindings]) =>
          (bindings ?? []).map((b) => `${b.HostIp || '0.0.0.0'}:${b.HostPort} → ${port}`).join(', ')
        )
    : [];

  const networks = Object.entries(container.NetworkSettings?.Networks ?? {});
  const exposedPorts = container.Config?.ExposedPorts
    ? Object.keys(container.Config.ExposedPorts)
    : [];
  const portMappings = editData?.portMappings ?? [];

  const dns = editing ? (editData?.dns ?? []) : (container.HostConfig?.Dns ?? []);
  const dnsSearch = editing ? (editData?.dnsSearch ?? []) : (container.HostConfig?.DnsSearch ?? []);
  const dnsOptions = editing ? (editData?.dnsOptions ?? []) : (container.HostConfig?.DnsOptions ?? []);
  const extraHosts = useMemo(
    () => (editing ? (editData?.extraHosts ?? []) : (container.HostConfig?.ExtraHosts ?? [])),
    [container.HostConfig?.ExtraHosts, editData?.extraHosts, editing],
  );
  const portMappingKeys = useStableListKeys(
    portMappings,
    (mapping) =>
      `${mapping.hostIp}\u0000${mapping.hostPort}\u0000${mapping.containerPort}\u0000${mapping.protocol}`,
  );
  const extraHostKeys = useStableListKeys(extraHosts, (host) => host);

  const handlePortMappingChange = useCallback(
    (index: number, field: keyof PortMappingEntry, value: string) => {
      const mappings = [...(editData?.portMappings ?? [])];
      const existing = mappings[index] ?? { containerPort: '', hostPort: '', hostIp: '0.0.0.0', protocol: 'tcp' };
      mappings[index] = { ...existing, [field]: value };
      onFieldChange?.('portMappings', mappings);
    },
    [editData?.portMappings, onFieldChange],
  );

  const handleAddPortMapping = useCallback(() => {
    onFieldChange?.('portMappings', [
      ...(editData?.portMappings ?? []),
      { containerPort: '', hostPort: '', hostIp: '0.0.0.0', protocol: 'tcp' },
    ]);
  }, [editData?.portMappings, onFieldChange]);

  const handleRemovePortMapping = useCallback(
    (index: number) => {
      onFieldChange?.('portMappings', (editData?.portMappings ?? []).filter((_, i) => i !== index));
    },
    [editData?.portMappings, onFieldChange],
  );

  const handleExtraHostChange = useCallback(
    (index: number, value: string) => {
      const hosts = [...extraHosts];
      hosts[index] = value;
      onFieldChange?.('extraHosts', hosts);
    },
    [extraHosts, onFieldChange],
  );

  const handleAddExtraHost = useCallback(() => {
    onFieldChange?.('extraHosts', [...extraHosts, '']);
  }, [extraHosts, onFieldChange]);

  const handleRemoveExtraHost = useCallback(
    (index: number) => {
      onFieldChange?.('extraHosts', extraHosts.filter((_, i) => i !== index));
    },
    [extraHosts, onFieldChange],
  );

  const joinableNetworks = (availableNetworks ?? []).filter((n: NetworkInfo) => !connectedNetworkNames.has(n.Name));

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <NetworkTabPortsSection
        editing={editing}
        exposedPorts={exposedPorts}
        portMappingKeys={portMappingKeys}
        portMappings={portMappings}
        ports={ports}
        onAddPortMapping={handleAddPortMapping}
        onPortMappingChange={handlePortMappingChange}
        onRemovePortMapping={handleRemovePortMapping}
      />
      <NetworkTabConfigSection
        connectMutation={connectMutation}
        container={container}
        disconnectMutation={disconnectMutation}
        dns={dns}
        dnsOptions={dnsOptions}
        dnsSearch={dnsSearch}
        editing={editing}
        editData={editData}
        extraHostKeys={extraHostKeys}
        extraHosts={extraHosts}
        joinableNetworks={joinableNetworks}
        networks={networks}
        onAddExtraHost={handleAddExtraHost}
        onExtraHostChange={handleExtraHostChange}
        onFieldChange={onFieldChange}
        onRemoveExtraHost={handleRemoveExtraHost}
        selectedNetwork={selectedNetwork}
        setSelectedNetwork={setSelectedNetwork}
      />
    </div>
  );
}
