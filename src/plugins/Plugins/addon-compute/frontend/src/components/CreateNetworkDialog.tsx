// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { useCreateNetwork, useUpdateNetwork } from '../hooks/useCompute';
import { useNetworkUplinks } from '../hooks/useNetworkUplinks';
import { HostChangePlan } from './HostChangePlan';
import { BridgedNetworkFields } from './BridgedNetworkFields';
import { DirectNetworkFields } from './DirectNetworkFields';
import { ExternalNetworkFields } from './ExternalNetworkFields';
import { ManagedNetworkFields } from './ManagedNetworkFields';
import { NetworkBasicsFields } from './NetworkBasicsFields';
import { NetworkDialogFooter } from './NetworkDialogFooter';
import { defaultHostLinkName } from '../utils/networkAttachment';
import type { HostLinkValues, Network, NetworkMode } from '../types';

const NO_HOST_LINK: HostLinkValues = {
  hostAccess: false,
  hostAccessIp: '',
  ipv6Enabled: false,
  hostAccessIpv6: '',
  hostLinkName: '',
};

/**
 * Add a guest network.
 *
 * Bridged is offered first and preselected, because it is both the common case
 * and the safe one: McHarbor creates the guest's own interface and nothing
 * else, so a mistake here cannot take the host's networking down.
 *
 * Managed mode creates its own bridge beside the host's adapters and never
 * touches them, so naming the interface guest traffic leaves by is routine.
 * External mode is the one that takes over an adapter, and the warning lives
 * there — where it is earned — rather than on every uplink picker.
 *
 * macvlan stacks the guest straight on an adapter without touching it, so it
 * is safe like bridged is, but it carries its own caveat: the host cannot
 * reach its own guests without the shim, and that warning lives in
 * DirectNetworkFields for the same reason.
 */

interface CreateNetworkDialogProps {
  environments: { id: string; name: string }[];
  defaultEnvironmentId?: string;
  /** The network being edited. Absent creates a new one. */
  network?: Network;
  onClose: () => void;
}

export function CreateNetworkDialog({
  environments,
  defaultEnvironmentId,
  network,
  onClose,
}: CreateNetworkDialogProps) {
  const { t } = useTranslation('compute');
  const editing = Boolean(network);

  // The host's topbar selection comes before whatever sorted first, which is
  // what this used to offer regardless of what the user had chosen.
  const hostEnvironmentId = useHostEnvironment();
  const [environmentId, setEnvironmentId] = useState(
    network?.environmentId || defaultEnvironmentId || hostEnvironmentId || environments[0]?.id || '',
  );
  const [networkName, setNetworkName] = useState(network?.name ?? '');
  const [mode, setMode] = useState<NetworkMode>((network?.mode as NetworkMode) ?? 'bridged');
  const [bridge, setBridge] = useState(network?.bridge ?? '');
  const [vlanTag, setVlanTag] = useState(network?.vlanTag ? String(network.vlanTag) : '');
  const [subnetCidr, setSubnetCidr] = useState(network?.subnetCidr ?? '10.100.0.0/24');
  const [gatewayIp, setGatewayIp] = useState(network?.gatewayIp ?? '10.100.0.1');
  const [dhcpEnabled, setDhcpEnabled] = useState(network?.dhcpEnabled ?? true);
  const [natEnabled, setNatEnabled] = useState(network?.natEnabled ?? true);
  const [uplinkLink, setUplinkLink] = useState(network?.uplinkLink ?? '');
  // An adopted adapter was accepted once already, so re-confirming it on every
  // edit would be a checkbox the operator learns to tick without reading.
  const [acceptExternal, setAcceptExternal] = useState(Boolean(network));
  const [linkMode, setLinkMode] = useState(network?.linkMode || 'bridge');
  const [hostLink, setHostLink] = useState<HostLinkValues>({
    hostAccess: network?.hostAccess ?? false,
    hostAccessIp: network?.hostAccessIp ?? '',
    ipv6Enabled: network?.ipv6Enabled ?? false,
    hostAccessIpv6: network?.hostAccessIpv6 ?? '',
    hostLinkName: network?.hostLinkName ?? '',
  });

  const create = useCreateNetwork();
  const update = useUpdateNetwork();
  const { isLoading, bridges, uplinks, uplinkIsDefaultRoute, uplinkBlockers, uplinkIsBusy, noHostNetwork } =
    useNetworkUplinks(environmentId, uplinkLink);

  const isDirect = mode === 'macvlan';

  const modeIsValid = isDirect
    ? Boolean(uplinkLink)
    : mode === 'bridged'
      ? Boolean(bridge)
      : mode === 'external'
        ? Boolean(uplinkLink) && acceptExternal && !uplinkIsBusy
        : Boolean(subnetCidr.trim() && gatewayIp.trim()) && (!natEnabled || Boolean(uplinkLink));

  const canSubmit = Boolean(environmentId && networkName.trim()) && modeIsValid;

  const submit = () => {
    if (!canSubmit) return;
    const body = {
      environmentId,
      name: networkName.trim(),
      mode,
      bridge: mode === 'bridged' ? bridge : undefined,
      vlanTag: vlanTag ? Number(vlanTag) : undefined,
      subnetCidr: mode === 'managed' ? subnetCidr.trim() : undefined,
      gatewayIp: mode === 'managed' ? gatewayIp.trim() : undefined,
      dhcpEnabled: mode === 'managed' ? dhcpEnabled : false,
      natEnabled: mode === 'managed' ? natEnabled : false,
      uplinkLink: mode === 'bridged' ? undefined : uplinkLink,
      linkMode: isDirect ? linkMode : undefined,
      hostAccess: isDirect ? hostLink.hostAccess : undefined,
      hostAccessIp: isDirect && hostLink.hostAccess ? hostLink.hostAccessIp.trim() || undefined : undefined,
      ipv6Enabled: isDirect && hostLink.hostAccess ? hostLink.ipv6Enabled : undefined,
      hostAccessIpv6: isDirect && hostLink.ipv6Enabled ? hostLink.hostAccessIpv6.trim() || undefined : undefined,
      hostLinkName: isDirect && hostLink.hostAccess ? hostLink.hostLinkName.trim() || undefined : undefined,
    };
    if (network) {
      update.mutate({ id: network.id, body }, { onSuccess: onClose });
      return;
    }
    create.mutate(body, { onSuccess: onClose });
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editing ? t('networks.edit.title', { name: network?.name }) : t('networks.create.title')}
          </DialogTitle>
          <DialogDescription>{editing ? t('networks.edit.subtitle') : t('networks.subtitle')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <NetworkBasicsFields
            environments={environments}
            environmentId={environmentId}
            onEnvironmentChange={(value) => {
              setEnvironmentId(value);
              setBridge('');
              setUplinkLink('');
            }}
            editing={editing}
            networkName={networkName}
            onNameChange={setNetworkName}
            mode={mode}
            onModeChange={(next) => {
              setMode(next);
              setUplinkLink('');
              setAcceptExternal(false);
              setHostLink(NO_HOST_LINK);
              setLinkMode('bridge');
            }}
          />

          {isDirect ? (
            <DirectNetworkFields
              mode={mode}
              parents={uplinks}
              parentsLoading={isLoading}
              noHostNetwork={noHostNetwork}
              uplinkLink={uplinkLink}
              onUplinkChange={setUplinkLink}
              linkMode={linkMode}
              onLinkModeChange={setLinkMode}
              vlanTag={vlanTag}
              onVlanTagChange={setVlanTag}
              hostLink={hostLink}
              onHostLinkChange={setHostLink}
              defaultHostLinkName={defaultHostLinkName(network?.id)}
            />
          ) : mode === 'bridged' ? (
            <BridgedNetworkFields
              bridges={bridges}
              isLoading={isLoading}
              noHostNetwork={noHostNetwork}
              bridge={bridge}
              onBridgeChange={setBridge}
              vlanTag={vlanTag}
              onVlanTagChange={setVlanTag}
            />
          ) : mode === 'managed' ? (
            <ManagedNetworkFields
              uplinks={uplinks}
              isLoading={isLoading}
              noHostNetwork={noHostNetwork}
              subnetCidr={subnetCidr}
              onSubnetCidrChange={setSubnetCidr}
              gatewayIp={gatewayIp}
              onGatewayIpChange={setGatewayIp}
              dhcpEnabled={dhcpEnabled}
              onDhcpEnabledChange={setDhcpEnabled}
              natEnabled={natEnabled}
              onNatEnabledChange={setNatEnabled}
              uplinkLink={uplinkLink}
              onUplinkChange={setUplinkLink}
            />
          ) : null}

          {mode === 'external' && (
            <ExternalNetworkFields
              uplinks={uplinks}
              isLoading={isLoading}
              noHostNetwork={noHostNetwork}
              uplinkLink={uplinkLink}
              onUplinkChange={setUplinkLink}
              uplinkBlockers={uplinkBlockers}
              uplinkIsDefaultRoute={uplinkIsDefaultRoute}
              acceptExternal={acceptExternal}
              onAcceptExternalChange={setAcceptExternal}
            />
          )}

          <HostChangePlan
            mode={mode}
            bridge={bridge}
            vlanTag={vlanTag}
            subnetCidr={subnetCidr}
            gatewayIp={gatewayIp}
            dhcpEnabled={dhcpEnabled}
            natEnabled={natEnabled}
            uplinkLink={uplinkLink}
            linkMode={linkMode}
            hostAccess={hostLink.hostAccess}
            hostAccessIp={hostLink.hostAccessIp}
            ipv6Enabled={hostLink.ipv6Enabled}
            hostAccessIpv6={hostLink.hostAccessIpv6}
            hostLinkName={hostLink.hostLinkName}
          />
        </div>

        <NetworkDialogFooter
          editing={editing}
          canSubmit={canSubmit}
          pending={create.isPending || update.isPending}
          onClose={onClose}
          onSubmit={submit}
        />
      </DialogContent>
    </Dialog>
  );
}
