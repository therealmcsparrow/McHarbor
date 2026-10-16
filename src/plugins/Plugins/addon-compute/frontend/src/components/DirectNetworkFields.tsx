// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import type { HostLink, HostLinkValues, NetworkMode } from '../types';
import { HostLinkFields } from './HostLinkFields';

const MACVLAN_MODES = ['bridge', 'vepa', 'private', 'passthru'] as const;

interface DirectNetworkFieldsProps {
  mode: NetworkMode;
  parents: HostLink[];
  parentsLoading: boolean;
  noHostNetwork: boolean;
  uplinkLink: string;
  onUplinkChange: (value: string) => void;
  linkMode: string;
  onLinkModeChange: (value: string) => void;
  vlanTag: string;
  onVlanTagChange: (value: string) => void;
  hostLink: HostLinkValues;
  onHostLinkChange: (value: HostLinkValues) => void;
  defaultHostLinkName: string;
}

export function DirectNetworkFields({
  mode,
  parents,
  parentsLoading,
  noHostNetwork,
  uplinkLink,
  onUplinkChange,
  linkMode,
  onLinkModeChange,
  vlanTag,
  onVlanTagChange,
  hostLink,
  onHostLinkChange,
  defaultHostLinkName,
}: DirectNetworkFieldsProps) {
  const { t } = useTranslation('compute');

  const modes = MACVLAN_MODES;
  const passthru = linkMode === 'passthru';

  return (
    <>
      <div>
        <Label>{t('networks.create.parent')}</Label>
        {parentsLoading ? (
          <Spinner className="h-4 w-4" />
        ) : parents.length > 0 ? (
          <Select
            value={uplinkLink}
            ariaLabel={t('networks.create.parent')}
            placeholder={t('networks.create.parentPlaceholder')}
            options={parents.map((link) => ({
              value: link.name,
              label: link.addresses?.length ? `${link.name} — ${link.addresses[0]}` : link.name,
            }))}
            onChange={onUplinkChange}
          />
        ) : (
          <p className="text-sm text-warning">
            {noHostNetwork ? t('networks.create.noHostNetwork') : t('networks.create.noUplinks')}
          </p>
        )}
        <p className="mt-1 text-xs text-muted-foreground">{t('networks.create.parentHint')}</p>
      </div>

      <div>
        <Label>{t('networks.create.linkMode')}</Label>
        <Select
          value={linkMode}
          ariaLabel={t('networks.create.linkMode')}
          options={modes.map((value) => ({
            value,
            label: t(`networks.linkMode.${mode}.${value}`),
          }))}
          onChange={onLinkModeChange}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t(`networks.linkMode.${mode}.${linkMode}Hint`)}</p>
      </div>

      <div>
        <Label htmlFor="net-direct-vlan">{t('networks.create.vlan')}</Label>
        <NumberInput
          id="net-direct-vlan"
          nullable
          min={1}
          max={4094}
          value={vlanTag.trim() === '' ? null : Number(vlanTag)}
          onChange={(next) => onVlanTagChange(next === null ? '' : String(next))}
        />
        <p className="mt-1 text-xs text-muted-foreground">
          {t('networks.create.directVlanHint', {
            parent: uplinkLink || t('networks.create.parent'),
          })}
        </p>
      </div>

      <div className="flex items-start gap-2 rounded-md bg-warning/10 p-2">
        <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
        <p className="text-sm text-foreground">
          {t('networks.directHostWarning', {
            parent: uplinkLink || t('networks.create.parent'),
          })}
        </p>
      </div>

      <HostLinkFields
        value={hostLink}
        onChange={onHostLinkChange}
        passthru={passthru}
        defaultName={defaultHostLinkName}
      />
    </>
  );
}
