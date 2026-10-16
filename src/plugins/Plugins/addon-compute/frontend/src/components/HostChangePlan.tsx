// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconServerCog } from '@tabler/icons-react';
import type { NetworkMode } from '../types';

interface HostChangePlanProps {
  mode: NetworkMode;
  bridge?: string;
  vlanTag?: string;
  subnetCidr?: string;
  gatewayIp?: string;
  dhcpEnabled?: boolean;
  natEnabled?: boolean;
  uplinkLink?: string;
  linkMode?: string;
  hostAccess?: boolean;
  hostAccessIp?: string;
  ipv6Enabled?: boolean;
  hostAccessIpv6?: string;
  hostLinkName?: string;
}

export function HostChangePlan({
  mode,
  bridge,
  vlanTag,
  subnetCidr,
  gatewayIp,
  dhcpEnabled,
  natEnabled,
  uplinkLink,
  linkMode,
  hostAccess,
  hostAccessIp,
  ipv6Enabled,
  hostAccessIpv6,
  hostLinkName,
}: HostChangePlanProps) {
  const { t } = useTranslation('compute');

  const steps: string[] = [];
  const cautions: string[] = [];
  let removal = '';

  if (mode === 'bridged') {
    if (!bridge) return null;
    steps.push(t('networks.plan.bridgedCreatesNothing', { bridge }));
    steps.push(t('networks.plan.bridgedTap', { bridge }));
    if (vlanTag) steps.push(t('networks.plan.bridgedVlan', { bridge, vlan: vlanTag }));
    removal = t('networks.plan.bridgedRemove');
  }

  if (mode === 'managed') {
    if (!subnetCidr || !gatewayIp) return null;
    steps.push(t('networks.plan.managedBridge'));
    steps.push(t('networks.plan.managedAddress', { gateway: gatewayIp, subnet: subnetCidr }));
    if (dhcpEnabled) steps.push(t('networks.plan.managedDhcp', { subnet: subnetCidr }));
    if (natEnabled && uplinkLink) {
      steps.push(t('networks.plan.managedNat', { subnet: subnetCidr, uplink: uplinkLink }));
      steps.push(t('networks.plan.managedUplinkUntouched', { uplink: uplinkLink }));
    }
    steps.push(t('networks.plan.managedTap'));
    removal = t('networks.plan.managedRemove');
  }

  if (mode === 'external') {
    if (!uplinkLink) return null;
    steps.push(t('networks.plan.externalBridge', { adapter: uplinkLink }));
    steps.push(t('networks.plan.externalEnslave', { adapter: uplinkLink }));
    steps.push(t('networks.plan.externalAddresses', { adapter: uplinkLink }));
    steps.push(t('networks.plan.externalRoutes', { adapter: uplinkLink }));
    steps.push(t('networks.plan.externalRollback'));
    steps.push(t('networks.plan.externalFailure'));
    removal = t('networks.plan.externalRemove', { adapter: uplinkLink });
    cautions.push(t('networks.plan.externalCaveatManager', { adapter: uplinkLink }));
    cautions.push(t('networks.plan.externalCaveatIpv6', { adapter: uplinkLink }));
    cautions.push(t('networks.plan.externalCaveatStack', { adapter: uplinkLink }));
    cautions.push(t('networks.plan.externalCaveatSession', { adapter: uplinkLink }));
  }

  if (mode === 'macvlan') {
    if (!uplinkLink) return null;
    steps.push(t('networks.plan.directUntouched', { parent: uplinkLink }));
    if (vlanTag) steps.push(t('networks.plan.directVlan', { parent: uplinkLink, vlan: vlanTag }));
    steps.push(t(`networks.plan.directChild.${mode}`, { parent: uplinkLink }));
    steps.push(t('networks.plan.directUpstream'));
    if (hostAccess) {
      steps.push(
        hostAccessIp ? t('networks.plan.directShimAddress', { address: hostAccessIp }) : t('networks.plan.directShim'),
      );
      if (hostLinkName) steps.push(t('networks.plan.directShimNamed', { name: hostLinkName }));
      if (ipv6Enabled) {
        steps.push(
          hostAccessIpv6
            ? t('networks.plan.directShimIpv6', { address: hostAccessIpv6 })
            : t('networks.plan.directShimSlaac'),
        );
      }
    }
    removal = t('networks.plan.directRemove', { parent: uplinkLink });
    if (!hostAccess) cautions.push(t('networks.plan.directCaveatHost', { parent: uplinkLink }));
    if (linkMode !== 'bridge') {
      cautions.push(t('networks.plan.directCaveatIsolation'));
    }
  }

  if (steps.length === 0) return null;

  return (
    <section className="rounded-md border border-border bg-muted/30 p-3">
      <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
        <IconServerCog className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        {t('networks.plan.title')}
      </h3>

      <p className="mt-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t('networks.plan.stepsTitle')}
      </p>
      <ol className="mt-1 list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
        {steps.map((step) => (
          <li key={step}>{step}</li>
        ))}
      </ol>

      <p className="mt-3 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t('networks.plan.removeTitle')}
      </p>
      <p className="mt-1 text-sm text-muted-foreground">{removal}</p>

      {cautions.length > 0 && (
        <div className="mt-3 rounded-md bg-warning/10 p-2">
          <p className="flex items-center gap-2 text-xs font-medium tracking-wide text-warning uppercase">
            <IconAlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
            {t('networks.plan.cautionTitle')}
          </p>
          <ul className="mt-1 list-disc space-y-1 pl-5 text-sm text-foreground">
            {cautions.map((caution) => (
              <li key={caution}>{caution}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
