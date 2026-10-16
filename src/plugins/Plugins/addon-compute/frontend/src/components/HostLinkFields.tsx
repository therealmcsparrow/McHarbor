// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import type { HostLinkValues } from '../types';

interface HostLinkFieldsProps {
  value: HostLinkValues;
  onChange: (value: HostLinkValues) => void;
  passthru: boolean;
  /** The name the agent generates when the field is left empty, shown as the placeholder; empty while the network has no id yet. */
  defaultName: string;
}

/**
 * The host's own interface on a direct network: whether it exists, what it is
 * called, and its IPv4 and IPv6 addresses. Turning a parent switch off clears
 * what hangs off it, so no hidden value is sent with the request.
 */
export function HostLinkFields({ value, onChange, passthru, defaultName }: HostLinkFieldsProps) {
  const { t } = useTranslation('compute');

  const setHostAccess = (hostAccess: boolean) =>
    onChange(
      hostAccess
        ? { ...value, hostAccess }
        : { hostAccess, hostAccessIp: '', ipv6Enabled: false, hostAccessIpv6: '', hostLinkName: '' },
    );
  const setIpv6 = (ipv6Enabled: boolean) =>
    onChange({ ...value, ipv6Enabled, hostAccessIpv6: ipv6Enabled ? value.hostAccessIpv6 : '' });

  return (
    <>
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor="net-host-access" className="text-sm font-normal">
          {t('networks.create.hostAccess')}
        </Label>
        <Switch id="net-host-access" checked={value.hostAccess} disabled={passthru} onCheckedChange={setHostAccess} />
      </div>
      {passthru && <p className="text-sm text-warning">{t('networks.create.hostAccessPassthru')}</p>}

      {value.hostAccess && !passthru && (
        <>
          <div>
            <Label htmlFor="net-host-link-name">{t('networks.create.hostLinkName')}</Label>
            <Input
              id="net-host-link-name"
              value={value.hostLinkName}
              onChange={(ev) => onChange({ ...value, hostLinkName: ev.target.value })}
              placeholder={defaultName || t('networks.create.hostLinkNameAuto')}
              maxLength={15}
              autoComplete="off"
              spellCheck={false}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.hostLinkNameHint')}</p>
          </div>

          <div>
            <Label htmlFor="net-host-access-ip">{t('networks.create.hostAccessIp')}</Label>
            <Input
              id="net-host-access-ip"
              value={value.hostAccessIp}
              onChange={(ev) => onChange({ ...value, hostAccessIp: ev.target.value })}
              placeholder="192.168.1.9/24"
              autoComplete="off"
              spellCheck={false}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.hostAccessIpHint')}</p>
          </div>

          <div className="flex items-center justify-between gap-3">
            <div>
              <Label htmlFor="net-host-ipv6" className="text-sm font-normal">
                {t('networks.create.ipv6')}
              </Label>
              <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.ipv6Hint')}</p>
            </div>
            <Switch id="net-host-ipv6" checked={value.ipv6Enabled} onCheckedChange={setIpv6} />
          </div>

          {value.ipv6Enabled && (
            <div>
              <Label htmlFor="net-host-access-ipv6">{t('networks.create.hostAccessIpv6')}</Label>
              <Input
                id="net-host-access-ipv6"
                value={value.hostAccessIpv6}
                onChange={(ev) => onChange({ ...value, hostAccessIpv6: ev.target.value })}
                // eslint-disable-next-line i18next/no-literal-string -- an example IPv6 address is a technical value, not prose
                placeholder="fd00::9/64"
                autoComplete="off"
                spellCheck={false}
              />
              <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.hostAccessIpv6Hint')}</p>
            </div>
          )}
        </>
      )}
    </>
  );
}
