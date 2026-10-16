// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { COMPUTE_PERM } from '../../permissions';
import { useCapability, useNetworks, usePools, useUpdateEnvironment } from '../../hooks/useCompute';
import type { OverviewEntry } from '../../types';

interface HostTogglesSectionProps {
  entry: OverviewEntry;
}

export function HostTogglesSection({ entry }: HostTogglesSectionProps) {
  const { t } = useTranslation('compute');
  const { denied, reason } = usePermissionGate(COMPUTE_PERM.MANAGE);
  const update = useUpdateEnvironment();
  const { data: env } = useCapability(entry.environmentId);
  const { data: pools } = usePools(entry.environmentId);
  const { data: networks } = useNetworks(entry.environmentId);

  const toggles = [
    {
      key: 'computeEnabled',
      checked: entry.computeEnabled,
      onChange: (next: boolean) => update.mutate({ envId: entry.environmentId, computeEnabled: next }),
    },
    {
      key: 'sharedLibrary',
      checked: Boolean(entry.sharedLibrary),
      onChange: (next: boolean) => update.mutate({ envId: entry.environmentId, sharedLibrary: next }),
    },
  ];

  return (
    <Card className="space-y-5 p-5">
      <h2 className="text-sm font-medium text-foreground">{t('hosts.settings.general')}</h2>

      {toggles.map((toggle) => (
        <div key={toggle.key} className="flex items-center justify-between gap-4">
          <div className="min-w-0">
            <p className="text-sm text-foreground">{t(`hosts.settings.${toggle.key}`)}</p>
            <p className="text-xs text-muted-foreground">{t(`hosts.settings.${toggle.key}Hint`)}</p>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex">
                <Switch
                  aria-label={t(`hosts.settings.${toggle.key}`)}
                  checked={toggle.checked}
                  disabled={denied || update.isPending}
                  onCheckedChange={toggle.onChange}
                />
              </span>
            </TooltipTrigger>
            <TooltipContent>{denied ? reason : t(`hosts.settings.${toggle.key}Hint`)}</TooltipContent>
          </Tooltip>
        </div>
      ))}

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <p className="text-sm text-foreground">{t('hosts.settings.defaultPool')}</p>
          <Select
            value={env?.defaultPoolId ?? ''}
            disabled={denied || update.isPending}
            placeholder={t('hosts.settings.nonePicked')}
            ariaLabel={t('hosts.settings.defaultPool')}
            options={(pools ?? []).map((pool) => ({
              value: pool.id,
              label: pool.name,
            }))}
            onChange={(value) =>
              update.mutate({
                envId: entry.environmentId,
                defaultPoolId: value,
              })
            }
          />
          <p className="text-xs text-muted-foreground">{t('hosts.settings.defaultPoolHint')}</p>
        </div>
        <div className="space-y-1.5">
          <p className="text-sm text-foreground">{t('hosts.settings.defaultNetwork')}</p>
          <Select
            value={env?.defaultNetworkId ?? ''}
            disabled={denied || update.isPending}
            placeholder={t('hosts.settings.nonePicked')}
            ariaLabel={t('hosts.settings.defaultNetwork')}
            options={(networks ?? []).map((network) => ({
              value: network.id,
              label: network.name,
            }))}
            onChange={(value) =>
              update.mutate({
                envId: entry.environmentId,
                defaultNetworkId: value,
              })
            }
          />
          <p className="text-xs text-muted-foreground">{t('hosts.settings.defaultNetworkHint')}</p>
        </div>
      </div>
    </Card>
  );
}
