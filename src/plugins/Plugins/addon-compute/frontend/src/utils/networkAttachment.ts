// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Network } from '../types';

export function isDirectNetwork(mode: string): boolean {
  return mode === 'macvlan' || mode === 'ipvlan';
}

/** The name the agent gives the host interface when the operator names none: the last five characters of the network id, empty before the network has one. */
export function defaultHostLinkName(networkId?: string): string {
  return networkId ? `mcharbor-h${networkId.slice(-5)}` : '';
}

/** The host's own interface on a direct network, or '' when the host has none. */
export function hostLinkLabel(network: Network): string {
  if (!isDirectNetwork(network.mode) || !network.hostAccess) return '';
  const address = [network.hostAccessIp, network.ipv6Enabled ? network.hostAccessIpv6 || 'SLAAC' : '']
    .filter(Boolean)
    .join(' · ');
  const linkName = network.hostLinkName || defaultHostLinkName(network.id);
  return address ? `${linkName} · ${address}` : linkName;
}

export function attachmentLabel(network: Network): string {
  const base = isDirectNetwork(network.mode)
    ? [network.uplinkLink, network.linkMode].filter(Boolean).join(' · ')
    : network.bridge;
  if (!base) return '—';
  return network.vlanTag ? `${base} · VLAN ${network.vlanTag}` : base;
}
