// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash, IconLink, IconUnlink } from '@tabler/icons-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { useStableListKeys } from '@resources/hooks/useStableListKeys';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import type { EditFormData, PortMappingEntry } from '../../types/edit-form';

type NetworkTabProps = {
  container: ContainerInspect;
  editing?: boolean;
  editData?: EditFormData | null;
  onFieldChange?: <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;
};

export function NetworkTab({ container, editing = false, editData, onFieldChange }: NetworkTabProps) {
  const { t } = useTranslation('containers');
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
      {/* Port Mappings */}
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.portMappings')}</h3>
        {editing ? (
          <div className="space-y-2">
            {(editData?.portMappings ?? []).length > 0 && (
              <div className="grid grid-cols-[1fr_1fr_100px_80px_auto] gap-2 text-xs font-medium text-muted-foreground">
                <span>{t('networkTab.hostPort')}</span>
                <span>{t('networkTab.containerPort')}</span>
                <span>{t('networkTab.hostIp')}</span>
                <span>{t('networkTab.protocol')}</span>
                <span className="w-8" />
              </div>
            )}
            {portMappings.map((m, i) => (
              <div key={portMappingKeys[i]} className="grid grid-cols-[1fr_1fr_100px_80px_auto] gap-2">
                <input
                  type="text"
                  value={m.hostPort}
                  onChange={(e) => handlePortMappingChange(i, 'hostPort', e.target.value)}
                  placeholder="8080"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <input
                  type="text"
                  value={m.containerPort}
                  onChange={(e) => handlePortMappingChange(i, 'containerPort', e.target.value)}
                  placeholder="80"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <input
                  type="text"
                  value={m.hostIp}
                  onChange={(e) => handlePortMappingChange(i, 'hostIp', e.target.value)}
                  placeholder="0.0.0.0"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <select
                  value={m.protocol}
                  onChange={(e) => handlePortMappingChange(i, 'protocol', e.target.value)}
                  className="rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
                >
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => handleRemovePortMapping(i)}
                  aria-label="Remove port"
                  className="size-7 text-muted-foreground hover:text-red-500"
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={handleAddPortMapping} className="mt-1">
              <IconPlus className="mr-1 size-3.5" />
              {t('edit.addPort')}
            </Button>
          </div>
        ) : ports.length > 0 ? (
          <div className="space-y-2">
            {ports.map((p) => (
              <div key={p} className="rounded-md bg-muted px-3 py-2 font-mono text-xs text-foreground">
                {p}
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noPortMappings')}</p>
        )}
      </div>

      {/* Exposed Ports */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.exposedPorts')}</h3>
        {exposedPorts.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {exposedPorts.map((port) => (
              <Badge key={port} variant="secondary" className="font-mono text-xs">{port}</Badge>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noExposedPorts')}</p>
        )}
      </div>

      {/* Network Config */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.dnsConfig')}</h3>
        {editing ? (
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('networkTab.networkMode')}</label>
            <select
              value={editData?.networkMode ?? ''}
              onChange={(e) => onFieldChange?.('networkMode', e.target.value)}
              className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
            >
              <option value="bridge">bridge</option>
              <option value="host">host</option>
              <option value="none">none</option>
              {editData?.networkMode && !['bridge', 'host', 'none'].includes(editData.networkMode) && (
                <option value={editData.networkMode}>{editData.networkMode}</option>
              )}
            </select>
          </div>
        ) : (
          <>
            <InfoRow label={t('networkTab.hostname')}>{container.Config?.Hostname ?? '-'}</InfoRow>
            <InfoRow label={t('networkTab.networkMode')}>
              <Badge variant="secondary">{container.HostConfig?.NetworkMode ?? '-'}</Badge>
            </InfoRow>
          </>
        )}
      </div>

      {/* DNS */}
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.dns')}</h3>
        {editing ? (
          <div className="space-y-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsServers')}</label>
              <input
                type="text"
                value={dns.join(', ')}
                onChange={(e) =>
                  onFieldChange?.('dns', e.target.value ? e.target.value.split(',').map((s) => s.trim()) : [])
                }
                placeholder="8.8.8.8, 8.8.4.4"
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsSearch')}</label>
              <input
                type="text"
                value={dnsSearch.join(', ')}
                onChange={(e) =>
                  onFieldChange?.('dnsSearch', e.target.value ? e.target.value.split(',').map((s) => s.trim()) : [])
                }
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsOptionsLabel')}</label>
              <input
                type="text"
                value={dnsOptions.join(', ')}
                onChange={(e) =>
                  onFieldChange?.('dnsOptions', e.target.value ? e.target.value.split(',').map((s) => s.trim()) : [])
                }
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
          </div>
        ) : (
          <>
            <InfoRow label={t('networkTab.dnsServers')}>
              {dns.length > 0 ? dns.join(', ') : '-'}
            </InfoRow>
            <InfoRow label={t('networkTab.dnsSearch')}>
              {dnsSearch.length > 0 ? dnsSearch.join(', ') : '-'}
            </InfoRow>
            {dnsOptions.length > 0 && (
              <InfoRow label={t('networkTab.dnsOptionsLabel')}>
                {dnsOptions.join(', ')}
              </InfoRow>
            )}
          </>
        )}
      </div>

      {/* Extra Hosts */}
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.extraHosts')}</h3>
        {editing ? (
          <div className="space-y-2">
            {extraHosts.map((host, i) => (
              <div key={extraHostKeys[i]} className="flex gap-2">
                <input
                  type="text"
                  value={host}
                  onChange={(e) => handleExtraHostChange(i, e.target.value)}
                  placeholder="hostname:IP"
                  className="flex-1 rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => handleRemoveExtraHost(i)}
                  aria-label="Remove host"
                  className="size-7 text-muted-foreground hover:text-red-500"
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={handleAddExtraHost} className="mt-1">
              <IconPlus className="mr-1 size-3.5" />
              {t('networkTab.addExtraHost')}
            </Button>
          </div>
        ) : extraHosts.length > 0 ? (
          <div className="space-y-1">
            {extraHosts.map((host) => (
              <div key={host} className="rounded-md bg-muted px-3 py-1.5 font-mono text-xs text-foreground">{host}</div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noExtraHosts')}</p>
        )}
      </div>

      {/* Connected Networks */}
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.networks')}</h3>

        {/* Join network UI */}
        {!editing && joinableNetworks.length > 0 && (
          <div className="mb-4 flex items-center gap-2">
            <select
              value={selectedNetwork}
              onChange={(e) => setSelectedNetwork(e.target.value)}
              className="flex-1 rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
            >
              <option value="">{t('networkTab.selectNetwork')}</option>
              {joinableNetworks.map((n: NetworkInfo) => (
                <option key={n.Id} value={n.Name}>{n.Name}</option>
              ))}
            </select>
            <Button
              variant="outline"
              size="sm"
              onClick={() => selectedNetwork && connectMutation.mutate(selectedNetwork)}
              disabled={!selectedNetwork || connectMutation.isPending}
            >
              <IconLink className="mr-1 size-3.5" />
              {t('actions.connect')}
            </Button>
          </div>
        )}

        {networks.length > 0 ? (
          <div className="space-y-4">
            {networks.map(([netName, net]) => (
              <div key={netName} className="rounded-md border border-border p-4">
                <div className="mb-3 flex items-center justify-between">
                  <Badge variant="secondary">{netName}</Badge>
                  {!editing && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => disconnectMutation.mutate(netName)}
                      disabled={disconnectMutation.isPending}
                      className="text-red-500 hover:text-red-600"
                    >
                      <IconUnlink className="mr-1 size-3.5" />
                      {t('actions.disconnect')}
                    </Button>
                  )}
                </div>
                <div className="grid grid-cols-1 gap-x-8 sm:grid-cols-2">
                  <InfoRow label={t('networkTab.ipAddress')}>{net?.IPAddress || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.gateway')}>{net?.Gateway || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.macAddress')}>{net?.MacAddress || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.prefixLength')}>{net?.IPPrefixLen ?? '-'}</InfoRow>
                  {net?.GlobalIPv6Address && (
                    <>
                      <InfoRow label={t('networkTab.ipv6Address')}>{net.GlobalIPv6Address}</InfoRow>
                      <InfoRow label={t('networkTab.ipv6Gateway')}>{net.IPv6Gateway || '-'}</InfoRow>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noConnectedNetworks')}</p>
        )}
      </div>
    </div>
  );
}
