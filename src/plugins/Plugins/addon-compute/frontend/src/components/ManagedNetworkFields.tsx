// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import type { HostLink } from '../types';

interface ManagedNetworkFieldsProps {
  uplinks: HostLink[];
  isLoading: boolean;
  noHostNetwork: boolean;
  subnetCidr: string;
  onSubnetCidrChange: (value: string) => void;
  gatewayIp: string;
  onGatewayIpChange: (value: string) => void;
  dhcpEnabled: boolean;
  onDhcpEnabledChange: (value: boolean) => void;
  natEnabled: boolean;
  onNatEnabledChange: (value: boolean) => void;
  uplinkLink: string;
  onUplinkChange: (value: string) => void;
}

export function ManagedNetworkFields({
  uplinks,
  isLoading,
  noHostNetwork,
  subnetCidr,
  onSubnetCidrChange,
  gatewayIp,
  onGatewayIpChange,
  dhcpEnabled,
  onDhcpEnabledChange,
  natEnabled,
  onNatEnabledChange,
  uplinkLink,
  onUplinkChange,
}: ManagedNetworkFieldsProps) {
  const { t } = useTranslation('compute');

  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="net-subnet">{t('networks.create.subnet')}</Label>
          <Input
            id="net-subnet"
            value={subnetCidr}
            onChange={(ev) => onSubnetCidrChange(ev.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <div>
          <Label htmlFor="net-gateway">{t('networks.create.gateway')}</Label>
          <Input
            id="net-gateway"
            value={gatewayIp}
            onChange={(ev) => onGatewayIpChange(ev.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
      </div>

      <div className="flex items-center justify-between gap-3">
        <Label htmlFor="net-dhcp" className="text-sm font-normal">
          {t('networks.create.dhcp')}
        </Label>
        <Switch id="net-dhcp" checked={dhcpEnabled} onCheckedChange={onDhcpEnabledChange} />
      </div>

      <div className="flex items-center justify-between gap-3">
        <Label htmlFor="net-nat" className="text-sm font-normal">
          {t('networks.create.nat')}
        </Label>
        <Switch id="net-nat" checked={natEnabled} onCheckedChange={onNatEnabledChange} />
      </div>

      {natEnabled && (
        <div>
          <Label>{t('networks.create.uplink')}</Label>
          {isLoading ? (
            <Spinner className="h-4 w-4" />
          ) : uplinks.length > 0 ? (
            <Select
              value={uplinkLink}
              ariaLabel={t('networks.create.uplink')}
              placeholder={t('networks.create.uplinkPlaceholder')}
              options={uplinks.map((link) => ({
                value: link.name,
                label: link.hasDefaultRoute ? t('networks.create.uplinkDefaultRoute', { link: link.name }) : link.name,
              }))}
              onChange={onUplinkChange}
            />
          ) : (
            <p className="text-sm text-warning">
              {noHostNetwork ? t('networks.create.noHostNetwork') : t('networks.create.noUplinks')}
            </p>
          )}
          <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.uplinkHint')}</p>
        </div>
      )}
    </>
  );
}
