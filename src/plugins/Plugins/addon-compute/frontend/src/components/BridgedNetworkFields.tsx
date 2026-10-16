// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import type { HostLink } from '../types';

interface BridgedNetworkFieldsProps {
  bridges: HostLink[];
  isLoading: boolean;
  noHostNetwork: boolean;
  bridge: string;
  onBridgeChange: (value: string) => void;
  vlanTag: string;
  onVlanTagChange: (value: string) => void;
}

export function BridgedNetworkFields({
  bridges,
  isLoading,
  noHostNetwork,
  bridge,
  onBridgeChange,
  vlanTag,
  onVlanTagChange,
}: BridgedNetworkFieldsProps) {
  const { t } = useTranslation('compute');

  return (
    <>
      <div>
        <Label>{t('networks.create.bridge')}</Label>
        {isLoading ? (
          <Spinner className="h-4 w-4" />
        ) : bridges.length > 0 ? (
          <Select
            value={bridge}
            ariaLabel={t('networks.create.bridge')}
            placeholder={t('networks.create.bridgePlaceholder')}
            options={bridges.map((link) => ({
              value: link.name,
              label: link.addresses?.length ? `${link.name} — ${link.addresses[0]}` : link.name,
            }))}
            onChange={onBridgeChange}
          />
        ) : (
          <p className="text-sm text-warning">
            {noHostNetwork ? t('networks.create.noHostNetwork') : t('networks.create.noBridges')}
          </p>
        )}
      </div>

      <div>
        <Label htmlFor="net-vlan">{t('networks.create.vlan')}</Label>
        <NumberInput
          id="net-vlan"
          nullable
          min={1}
          max={4094}
          value={vlanTag.trim() === '' ? null : Number(vlanTag)}
          onChange={(next) => onVlanTagChange(next === null ? '' : String(next))}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.vlanHint')}</p>
      </div>
    </>
  );
}
