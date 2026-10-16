// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import type { HostLink } from '../types';

interface ExternalNetworkFieldsProps {
  uplinks: HostLink[];
  isLoading: boolean;
  noHostNetwork: boolean;
  uplinkLink: string;
  onUplinkChange: (value: string) => void;
  uplinkBlockers: string[];
  uplinkIsDefaultRoute: boolean;
  acceptExternal: boolean;
  onAcceptExternalChange: (value: boolean) => void;
}

export function ExternalNetworkFields({
  uplinks,
  isLoading,
  noHostNetwork,
  uplinkLink,
  onUplinkChange,
  uplinkBlockers,
  uplinkIsDefaultRoute,
  acceptExternal,
  onAcceptExternalChange,
}: ExternalNetworkFieldsProps) {
  const { t } = useTranslation('compute');
  const uplinkIsBusy = uplinkBlockers.length > 0;

  return (
    <>
      <div>
        <Label>{t('networks.create.adapter')}</Label>
        {isLoading ? (
          <Spinner className="h-4 w-4" />
        ) : uplinks.length > 0 ? (
          <Select
            value={uplinkLink}
            ariaLabel={t('networks.create.adapter')}
            placeholder={t('networks.create.adapterPlaceholder')}
            options={uplinks.map((link) => ({
              value: link.name,
              label: link.upper?.length
                ? t('networks.create.adapterBusyOption', { link: link.name })
                : link.addresses?.length
                  ? `${link.name} — ${link.addresses[0]}`
                  : link.name,
            }))}
            onChange={(value) => {
              onUplinkChange(value);
              onAcceptExternalChange(false);
            }}
          />
        ) : (
          <p className="text-sm text-warning">
            {noHostNetwork ? t('networks.create.noHostNetwork') : t('networks.create.noUplinks')}
          </p>
        )}
        <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.adapterHint')}</p>
        {uplinkIsBusy && (
          <p className="mt-1 text-sm text-warning">
            {t('networks.create.adapterBusy', {
              link: uplinkLink,
              interfaces: uplinkBlockers.join(', '),
            })}
          </p>
        )}
      </div>

      {uplinkLink && (
        <div className="flex items-start gap-2 rounded-md bg-destructive/10 p-2">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
          <p className="text-sm text-foreground">
            {uplinkIsDefaultRoute
              ? t('networks.externalWarningDefaultRoute', { link: uplinkLink })
              : t('networks.externalWarning', { link: uplinkLink })}
          </p>
        </div>
      )}

      {uplinkLink && (
        <div className="flex items-center justify-between gap-3">
          <Label htmlFor="net-accept-external" className="text-sm font-normal">
            {t('networks.create.acceptExternal')}
          </Label>
          <Switch id="net-accept-external" checked={acceptExternal} onCheckedChange={onAcceptExternalChange} />
        </div>
      )}
    </>
  );
}
