// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Zero-pads each IPv4 octet so a plain string compare orders addresses numerically
 * (10.0.0.9 before 10.0.0.10). Anything else is returned as-is; empty or "-" is null so it sorts last.
 */
export function ipv4SortKey(ip: string | null | undefined): string | null {
  if (!ip || ip === '-') return null;
  const octets = ip.split('.');
  if (octets.length !== 4 || octets.some((o) => !/^\d{1,3}$/.test(o))) return ip;
  return octets.map((o) => o.padStart(3, '0')).join('.');
}
