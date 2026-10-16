// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { CreateContainerForm } from './useCreateContainerForm';

/** Rendered only for a macvlan attachment: the agent has nowhere to apply a fixed v6 on a veth pair. */
export function StaticIPv6Fields({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const { staticIpv6, setStaticIpv6, staticGw6, setStaticGw6 } = form;

  return (
    <>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="ct-ipv6">{t('create.network.staticIpv6')}</Label>
          <Input id="ct-ipv6" value={staticIpv6} onChange={(ev) => setStaticIpv6(ev.target.value)} />
        </div>
        <div>
          <Label htmlFor="ct-gw6">{t('create.network.staticGateway6')}</Label>
          <Input id="ct-gw6" value={staticGw6} onChange={(ev) => setStaticGw6(ev.target.value)} />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{t('create.network.staticIpv6Hint')}</p>
    </>
  );
}
