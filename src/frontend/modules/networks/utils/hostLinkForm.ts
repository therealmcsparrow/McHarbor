// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { HostLinkRequest } from '@resources/hooks/networkHostLink';
import { parseHostLinkLabels } from './hostLinkLabels';

export type HostLinkFormValue = {
  enabled: boolean;
  name: string;
  ipv4: string;
  ipv6: string;
  ipv6Auto: boolean;
};

export const EMPTY_HOST_LINK: HostLinkFormValue = { enabled: false, name: '', ipv4: '', ipv6: '', ipv6Auto: false };

/** The form value for a network's existing host interface, read back from the labels the server wrote. */
export function hostLinkFormOf(labels: Record<string, string> | null | undefined): HostLinkFormValue {
  const link = parseHostLinkLabels(labels);
  if (!link) return EMPTY_HOST_LINK;
  return {
    enabled: true,
    name: link.name,
    ipv4: link.ipv4 ?? '',
    ipv6: link.ipv6 ?? '',
    ipv6Auto: link.ipv6Auto ?? false,
  };
}

/**
 * The request body for the form, or undefined when no host interface was asked for. IPv6 values are
 * dropped unless the network has IPv6 on, because the server refuses an address the network cannot carry.
 */
export function hostLinkRequestOf(value: HostLinkFormValue, enableIPv6: boolean): HostLinkRequest | undefined {
  if (!value.enabled) return undefined;
  const request: HostLinkRequest = { name: value.name.trim() };
  if (value.ipv4.trim()) request.ipv4 = value.ipv4.trim();
  if (enableIPv6 && value.ipv6Auto) request.ipv6Auto = true;
  else if (enableIPv6 && value.ipv6.trim()) request.ipv6 = value.ipv6.trim();
  return request;
}
