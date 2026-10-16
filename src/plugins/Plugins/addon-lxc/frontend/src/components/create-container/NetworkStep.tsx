// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { CreateContainerForm } from './useCreateContainerForm';
import { StaticIPv6Fields } from './StaticIPv6Fields';
import type { AddressMode } from '../../types';

export function NetworkStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const {
    networks,
    network,
    networkId,
    setNetworkId,
    addressMode,
    setAddressMode,
    staticIp,
    setStaticIp,
    staticGw,
    setStaticGw,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.network.title')}</h3>
      <div>
        <Label htmlFor="ct-network">{t('create.network.network')}</Label>
        <Select
          value={networkId}
          ariaLabel={t('create.network.network')}
          placeholder={t('create.network.networkPlaceholder')}
          options={(networks ?? []).map((entry) => ({
            value: entry.id,
            label: entry.name,
          }))}
          onChange={setNetworkId}
        />
        {network && (
          <p className="mt-1 text-xs text-muted-foreground">
            {t('create.network.attachment', {
              attachment:
                network.mode === 'macvlan' || network.mode === 'ipvlan'
                  ? `${network.mode} · ${network.uplinkLink ?? ''}`
                  : network.bridge,
            })}
          </p>
        )}
      </div>
      <div>
        <Label htmlFor="ct-address-mode">{t('create.network.addressMode')}</Label>
        <Select
          value={addressMode}
          ariaLabel={t('create.network.addressMode')}
          options={[
            { value: 'dhcp', label: t('create.network.dhcp') },
            { value: 'static', label: t('create.network.static') },
          ]}
          onChange={(value) => setAddressMode(value as AddressMode)}
        />
      </div>
      {addressMode === 'static' && (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label htmlFor="ct-ip">{t('create.network.staticIp')}</Label>
              <Input id="ct-ip" value={staticIp} onChange={(ev) => setStaticIp(ev.target.value)} />
            </div>
            <div>
              <Label htmlFor="ct-gw">{t('create.network.staticGateway')}</Label>
              <Input id="ct-gw" value={staticGw} onChange={(ev) => setStaticGw(ev.target.value)} />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">{t('create.network.staticHint')}</p>
        </>
      )}
      {form.macvlanIpv6 && <StaticIPv6Fields form={form} />}
    </>
  );
}
