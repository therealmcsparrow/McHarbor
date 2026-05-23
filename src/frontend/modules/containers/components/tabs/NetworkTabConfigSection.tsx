// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLink, IconPlus, IconTrash, IconUnlink } from '@tabler/icons-react';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { UseMutationResult } from '@tanstack/react-query';
import type { ContainerInspect, NetworkInfo } from '@core/types/docker';
import type { EditFormData } from '../../types/edit-form';

type NetworkTabConfigSectionProps = {
  connectMutation: UseMutationResult<unknown, Error, string, unknown>;
  container: ContainerInspect;
  disconnectMutation: UseMutationResult<unknown, Error, string, unknown>;
  dns: string[];
  dnsOptions: string[];
  dnsSearch: string[];
  editing: boolean;
  editData?: EditFormData | null;
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

function splitCsvValue(value: string): string[] {
  return value ? value.split(',').map((item) => item.trim()) : [];
}

export function NetworkTabConfigSection({
  connectMutation,
  container,
  disconnectMutation,
  dns,
  dnsOptions,
  dnsSearch,
  editing,
  editData,
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

  return (
    <>
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.dnsConfig')}</h3>
        {editing ? (
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('networkTab.networkMode')}</label>
            <select
              value={editData?.networkMode ?? ''}
              onChange={(event) => onFieldChange?.('networkMode', event.target.value)}
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

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.dns')}</h3>
        {editing ? (
          <div className="space-y-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsServers')}</label>
              <input
                type="text"
                value={dns.join(', ')}
                onChange={(event) => onFieldChange?.('dns', splitCsvValue(event.target.value))}
                placeholder="8.8.8.8, 8.8.4.4"
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsSearch')}</label>
              <input
                type="text"
                value={dnsSearch.join(', ')}
                onChange={(event) => onFieldChange?.('dnsSearch', splitCsvValue(event.target.value))}
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">{t('networkTab.dnsOptionsLabel')}</label>
              <input
                type="text"
                value={dnsOptions.join(', ')}
                onChange={(event) => onFieldChange?.('dnsOptions', splitCsvValue(event.target.value))}
                className="mt-1 w-full rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
              />
            </div>
          </div>
        ) : (
          <>
            <InfoRow label={t('networkTab.dnsServers')}>{dns.length > 0 ? dns.join(', ') : '-'}</InfoRow>
            <InfoRow label={t('networkTab.dnsSearch')}>{dnsSearch.length > 0 ? dnsSearch.join(', ') : '-'}</InfoRow>
            {dnsOptions.length > 0 && <InfoRow label={t('networkTab.dnsOptionsLabel')}>{dnsOptions.join(', ')}</InfoRow>}
          </>
        )}
      </div>

      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.extraHosts')}</h3>
        {editing ? (
          <div className="space-y-2">
            {extraHosts.map((host, index) => (
              <div key={extraHostKeys[index]} className="flex gap-2">
                <input
                  type="text"
                  value={host}
                  onChange={(event) => onExtraHostChange(index, event.target.value)}
                  placeholder="hostname:IP"
                  className="flex-1 rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => onRemoveExtraHost(index)}
                  aria-label="Remove host"
                  className="size-7 text-muted-foreground hover:text-red-500"
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={onAddExtraHost} className="mt-1">
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

      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.networks')}</h3>

        {!editing && joinableNetworks.length > 0 && (
          <div className="mb-4 flex items-center gap-2">
            <select
              value={selectedNetwork}
              onChange={(event) => setSelectedNetwork(event.target.value)}
              className="flex-1 rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
            >
              <option value="">{t('networkTab.selectNetwork')}</option>
              {joinableNetworks.map((network) => (
                <option key={network.Id} value={network.Name}>{network.Name}</option>
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
            {networks.map(([networkName, network]) => (
              <div key={networkName} className="rounded-md border border-border p-4">
                <div className="mb-3 flex items-center justify-between">
                  <Badge variant="secondary">{networkName}</Badge>
                  {!editing && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => disconnectMutation.mutate(networkName)}
                      disabled={disconnectMutation.isPending}
                      className="text-red-500 hover:text-red-600"
                    >
                      <IconUnlink className="mr-1 size-3.5" />
                      {t('actions.disconnect')}
                    </Button>
                  )}
                </div>
                <div className="grid grid-cols-1 gap-x-8 sm:grid-cols-2">
                  <InfoRow label={t('networkTab.ipAddress')}>{network?.IPAddress || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.gateway')}>{network?.Gateway || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.macAddress')}>{network?.MacAddress || '-'}</InfoRow>
                  <InfoRow label={t('networkTab.prefixLength')}>{network?.IPPrefixLen ?? '-'}</InfoRow>
                  {network?.GlobalIPv6Address && (
                    <>
                      <InfoRow label={t('networkTab.ipv6Address')}>{network.GlobalIPv6Address}</InfoRow>
                      <InfoRow label={t('networkTab.ipv6Gateway')}>{network.IPv6Gateway || '-'}</InfoRow>
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
    </>
  );
}
