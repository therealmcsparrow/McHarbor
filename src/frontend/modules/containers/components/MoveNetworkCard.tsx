// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { DRIVER_OPTIONS } from '@resources/constants/network-drivers';
import type { MoveContainerPlan, MoveNetworkConfig } from '../hooks/useContainers';
import { csv, splitCsv, updateIpam } from './move-network-settings-utils';

type MoveNetworkPlanItem = MoveContainerPlan['networks'][number] | undefined;

type MoveNetworkCardProps = {
  network: MoveNetworkConfig;
  source: MoveNetworkPlanItem;
  targetName: string;
  showCreationFields: boolean;
  showEndpointFields: boolean;
  updateNetwork: (sourceName: string, patch: Partial<MoveNetworkConfig>) => void;
  t: TFunction<'containers'>;
};

export function MoveNetworkCard({
  network,
  source,
  targetName,
  showCreationFields,
  showEndpointFields,
  updateNetwork,
  t,
}: MoveNetworkCardProps) {
  const ipam = network.ipam?.Config?.[0] ?? {};

  return (
    <div className="space-y-3 rounded-md border border-border bg-background/50 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-medium">
          {source?.name ?? network.sourceName}
          <span className="ml-2 text-xs text-muted-foreground">{t('moveDialog.to')}</span>
          <span className="ml-2 text-sm text-foreground">{targetName}</span>
        </div>
        <Badge variant={source?.willCreate ? 'warning' : 'secondary'}>
          {source?.willCreate ? t('moveDialog.create') : t('moveDialog.keep')}
        </Badge>
      </div>

      {showCreationFields && (
        <div className="grid gap-3 md:grid-cols-3">
          <div className="space-y-1.5">
            <Label>{t('moveDialog.driver')}</Label>
            <Select
              value={network.driver ?? 'bridge'}
              onChange={(value) => updateNetwork(network.sourceName, { driver: value })}
              options={[...DRIVER_OPTIONS]}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t('moveDialog.subnet')}</Label>
            <Input
              value={ipam.Subnet ?? ''}
              onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'Subnet', event.target.value))}
              placeholder="172.20.0.0/16"
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t('moveDialog.gateway')}</Label>
            <Input
              value={ipam.Gateway ?? ''}
              onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'Gateway', event.target.value))}
              placeholder="172.20.0.1"
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t('moveDialog.ipRange')}</Label>
            <Input
              value={ipam.IPRange ?? ''}
              onChange={(event) => updateNetwork(network.sourceName, updateIpam(network, 'IPRange', event.target.value))}
              placeholder="172.20.5.0/24"
            />
          </div>
        </div>
      )}

      {showEndpointFields && (
        <div className="grid gap-3 md:grid-cols-3">
          <div className="space-y-1.5">
            <Label>{t('moveDialog.aliases')}</Label>
            <Input
              value={csv(network.aliases)}
              onChange={(event) => updateNetwork(network.sourceName, { aliases: splitCsv(event.target.value) })}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t('moveDialog.targetIp')}</Label>
            <Input
              value={network.ipAddress ?? ''}
              onChange={(event) => updateNetwork(network.sourceName, { ipAddress: event.target.value })}
              placeholder={source?.ipAddress}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t('moveDialog.targetMac')}</Label>
            <Input
              value={network.macAddress ?? ''}
              onChange={(event) => updateNetwork(network.sourceName, { macAddress: event.target.value })}
              placeholder={source?.macAddress}
            />
          </div>
        </div>
      )}

      {showCreationFields && (
        <div className="flex flex-wrap gap-4">
          <Label className="flex items-center gap-2 text-sm">
            <Switch
              checked={!!network.internal}
              onCheckedChange={(checked) => updateNetwork(network.sourceName, { internal: checked })}
            />
            {t('moveDialog.internal')}
          </Label>
          <Label className="flex items-center gap-2 text-sm">
            <Switch
              checked={!!network.attachable}
              onCheckedChange={(checked) => updateNetwork(network.sourceName, { attachable: checked })}
            />
            {t('moveDialog.attachable')}
          </Label>
        </div>
      )}
    </div>
  );
}
