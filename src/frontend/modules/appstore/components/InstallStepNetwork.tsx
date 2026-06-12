// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconNetwork, IconRefresh } from '@tabler/icons-react';
import { api } from '@core/api/client';
import type { ContainerInfo, NetworkInfo } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AppNetworkMode, AppNetworkSettings } from '../types';

interface InstallStepNetworkProps {
  network: AppNetworkSettings;
  selectedEnvId: string;
  onNetworkChange: (value: AppNetworkSettings) => void;
}

const networkModes: Array<{ value: AppNetworkMode; labelKey: string }> = [
  { value: 'default', labelKey: 'appStore.networkModeDefault' },
  { value: 'bridge', labelKey: 'appStore.networkModeBridge' },
  { value: 'host', labelKey: 'appStore.networkModeHost' },
  { value: 'none', labelKey: 'appStore.networkModeNone' },
  { value: 'existing', labelKey: 'appStore.networkModeExisting' },
];

type InstalledNetworkOption = {
  name: string;
  driver: string;
};

function splitCsv(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

async function fetchInstalledNetworks(selectedEnvId: string): Promise<InstalledNetworkOption[]> {
  const params: Record<string, string> = selectedEnvId ? { env: selectedEnvId } : {};

  try {
    const networks = assertSuccess(await api.get<NetworkInfo[]>('/networks', params));
    return networks.map((item) => ({ name: item.Name, driver: item.Driver }));
  } catch (error) {
    const containers = assertSuccess(
      await api.get<ContainerInfo[]>('/containers', { ...params, all: 'true' }),
    );
    const names = new Set<string>();
    containers.forEach((container) => {
      Object.keys(container.NetworkSettings?.Networks ?? {}).forEach((networkName) => {
        names.add(networkName);
      });
    });
    if (names.size === 0) throw error;
    return [...names].map((name) => ({ name, driver: '' }));
  }
}

export function InstallStepNetwork({
  network,
  selectedEnvId,
  onNetworkChange,
}: InstallStepNetworkProps) {
  const { t } = useTranslation('common');
  const [aliasText, setAliasText] = useState(network.aliases.join(', '));

  const networksQuery = useQuery({
    queryKey: ['app-store-networks', selectedEnvId],
    queryFn: () => fetchInstalledNetworks(selectedEnvId),
    enabled: !!selectedEnvId && network.mode === 'existing',
    refetchInterval: 30_000,
  });

  const networkOptions = useMemo(
    () => (networksQuery.data ?? [])
      .map((item) => ({
        value: item.name,
        label: item.driver ? `${item.name} (${item.driver})` : item.name,
      }))
      .sort((a, b) => a.label.localeCompare(b.label)),
    [networksQuery.data],
  );

  const update = (patch: Partial<AppNetworkSettings>) => {
    onNetworkChange({ ...network, ...patch });
  };

  useEffect(() => {
    const firstNetwork = networkOptions[0]?.value;
    if (network.mode !== 'existing' || network.name || !firstNetwork) return;
    onNetworkChange({ ...network, name: firstNetwork });
  }, [network, networkOptions, onNetworkChange]);

  const handleModeChange = (mode: string) => {
    const nextMode = mode as AppNetworkMode;
    update({
      mode: nextMode,
      ...(nextMode !== 'existing'
        ? { name: '', aliases: [], ipv4Address: '', ipv6Address: '', macAddress: '' }
        : { name: networkOptions[0]?.value ?? network.name }),
    });
    if (nextMode !== 'existing') {
      setAliasText('');
    }
  };

  const handleAliasesChange = (value: string) => {
    setAliasText(value);
    update({ aliases: splitCsv(value) });
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        <IconNetwork className="size-4 text-primary" />
        {t('appStore.networkSettings')}
      </div>

      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">
          {t('appStore.networkMode')}
        </label>
        <Select
          value={network.mode}
          onChange={handleModeChange}
          options={networkModes.map((mode) => ({ value: mode.value, label: t(mode.labelKey) }))}
          searchable={false}
          ariaLabel={t('appStore.networkMode')}
        />
      </div>

      {network.mode === 'existing' && (
        <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
          <div>
            <div className="mb-1 flex items-center justify-between gap-2">
              <label className="block text-xs font-medium text-muted-foreground">
                {t('appStore.existingNetwork')}
              </label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 gap-1 px-2 text-xs"
                onClick={() => void networksQuery.refetch()}
                disabled={!selectedEnvId || networksQuery.isFetching}
              >
                <IconRefresh className="size-3.5" />
                {t('actions.refresh')}
              </Button>
            </div>
            <div className="flex gap-2">
              <Select
                value={networkOptions.some((option) => option.value === network.name) ? network.name : ''}
                onChange={(value) => update({ name: value })}
                options={networkOptions}
                placeholder={
                  networksQuery.isLoading || networksQuery.isFetching
                    ? t('appStore.loadingNetworks')
                    : t('appStore.selectNetwork')
                }
                disabled={networksQuery.isLoading || networksQuery.isFetching || networkOptions.length === 0}
                ariaLabel={t('appStore.existingNetwork')}
              />
            </div>
            {networksQuery.isError && (
              <p className="mt-1 text-xs text-destructive">{t('appStore.networkListError')}</p>
            )}
            {!networksQuery.isLoading && !networksQuery.isError && networkOptions.length === 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{t('appStore.noConfiguredNetworks')}</p>
            )}
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {networkOptions.length > 0
                ? t('appStore.manualNetworkOverride')
                : t('appStore.networkName')}
            </label>
            <input
              type="text"
              value={network.name}
              onChange={(event) => update({ name: event.target.value })}
              placeholder="frontend_net"
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('appStore.networkAliases')}
            </label>
            <input
              type="text"
              value={aliasText}
              onChange={(event) => handleAliasesChange(event.target.value)}
              placeholder="app, app.local"
              className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('appStore.ipv4Address')}
              </label>
              <input
                type="text"
                value={network.ipv4Address}
                onChange={(event) => update({ ipv4Address: event.target.value })}
                placeholder="172.20.0.10"
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('appStore.ipv6Address')}
              </label>
              <input
                type="text"
                value={network.ipv6Address}
                onChange={(event) => update({ ipv6Address: event.target.value })}
                placeholder="fd00::10"
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('appStore.macAddress')}
              </label>
              <input
                type="text"
                value={network.macAddress}
                onChange={(event) => update({ macAddress: event.target.value })}
                placeholder="02:42:ac:14:00:10"
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
