// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { HostLinkRequest } from '@resources/hooks/networkHostLink';

const PREFIX = 'com.mcharbor.host-link.';

/** The server owns these labels: it strips them from input and writes them from `hostLink`. */
export function isHostLinkLabel(key: string): boolean {
  return key.startsWith(PREFIX);
}

/** Reads the host link a network's labels record, or null when they name none. */
export function parseHostLinkLabels(labels: Record<string, string> | null | undefined): HostLinkRequest | null {
  const linkName = labels?.[`${PREFIX}name`]?.trim();
  if (!linkName) return null;
  const link: HostLinkRequest = { name: linkName };
  const ipv4 = labels?.[`${PREFIX}ipv4`];
  if (ipv4) link.ipv4 = ipv4;
  const ipv6 = labels?.[`${PREFIX}ipv6`];
  if (ipv6 === 'auto') link.ipv6Auto = true;
  else if (ipv6) link.ipv6 = ipv6;
  return link;
}

/** Drops the server-owned labels from an editable list, so a recreate cannot send them back as forged input. */
export function withoutHostLinkLabels<T extends { key: string }>(entries: T[]): T[] {
  return entries.filter((entry) => !isHostLinkLabel(entry.key));
}

const AUX_NAME = 'mcharbor-host-link';

/** The server re-reserves the host link's address from `hostLink`, so the old reservation must not ride along into a recreate. */
export function withoutHostLinkAddress(aux: Record<string, string> | undefined): Record<string, string> | undefined {
  if (!aux) return undefined;
  return Object.fromEntries(Object.entries(aux).filter(([auxName]) => auxName !== AUX_NAME));
}
