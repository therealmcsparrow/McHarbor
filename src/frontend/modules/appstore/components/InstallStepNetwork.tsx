// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconNetwork, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { AppNetworkMode, AppNetworkSettings } from '../types';
import { fetchInstalledNetworks, splitCsv } from './installNetworkData';

const MAC_PLACEHOLDER = '02:42:ac:14:00:10';

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

export function InstallStepNetwork({ network, selectedEnvId, onNetworkChange }: InstallStepNetworkProps) {
  const { t } = useTranslation('common');
  const [aliasText, setAliasText] = useState(network.aliases.join(', '));

  const networksQuery = useQuery({
    queryKey: ['app-store-networks', selectedEnvId],
    queryFn: () => fetchInstalledNetworks(selectedEnvId),
    enabled: !!selectedEnvId && network.mode === 'existing',
    refetchInterval: 30_000,
  });

  const networkOptions = useMemo(
    () =>
      (networksQuery.data ?? [])
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
        <label className="mb-1 block text-sm font-medium text-foreground">{t('appStore.networkMode')}</label>
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
              <label className="block text-xs font-medium text-muted-foreground">{t('appStore.existingNetwork')}</label>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="gap-1 px-2 text-xs"
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
              <p className="mt-1 text-xs text-destructive">
                {[t('appStore.networkListError'), networksQuery.error?.message].filter(Boolean).join(' — ')}
              </p>
            )}
            {!networksQuery.isLoading && !networksQuery.isError && networkOptions.length === 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{t('appStore.noConfiguredNetworks')}</p>
            )}
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {networkOptions.length > 0 ? t('appStore.manualNetworkOverride') : t('appStore.networkName')}
            </label>
            <input
              type="text"
              value={network.name}
              onChange={(evt2) => update({ name: evt2.target.value })}
              placeholder={t('placeholders.networkName')}
              className="mc-control w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
            />
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('appStore.networkAliases')}
            </label>
            <input
              type="text"
              value={aliasText}
              onChange={(evt2) => handleAliasesChange(evt2.target.value)}
              placeholder={t('placeholders.networkHostAliases')}
              className="mc-control w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
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
                onChange={(entry) => update({ ipv4Address: entry.target.value })}
                placeholder="172.20.0.10"
                className="mc-control w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('appStore.ipv6Address')}
              </label>
              <input
                type="text"
                value={network.ipv6Address}
                onChange={(eventItem) => update({ ipv6Address: eventItem.target.value })}
                placeholder={t('placeholders.ipv6Address')}
                className="mc-control w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">{t('appStore.macAddress')}</label>
              <input
                type="text"
                value={network.macAddress}
                onChange={(evt) => update({ macAddress: evt.target.value })}
                placeholder={MAC_PLACEHOLDER}
                className="mc-control w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary focus:outline-none"
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
