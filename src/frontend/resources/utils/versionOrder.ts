// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const STRICT_VERSION =
  /^([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

type ParsedVersion = { core: bigint[]; pre: string[] };

// Unparsable input reads as 0.0.0, the same as core/versionorder.Compare, so a
// dev host or a tag such as "latest" sorts below every real version.
function parseVersion(v: string): ParsedVersion {
  const m = STRICT_VERSION.exec(v);
  if (!m) return { core: [0n, 0n, 0n], pre: [] };
  return { core: [BigInt(m[1]!), BigInt(m[2]!), BigInt(m[3]!)], pre: m[4] ? m[4].split('.') : [] };
}

const isNumericIdent = (s: string) => /^[0-9]+$/.test(s);

function comparePre(a: string[], b: string[]): number {
  if (a.length === 0 || b.length === 0) return a.length === b.length ? 0 : a.length === 0 ? 1 : -1;
  for (let i = 0; i < Math.min(a.length, b.length); i++) {
    const x = a[i]!;
    const y = b[i]!;
    if (x === y) continue;
    const xn = isNumericIdent(x);
    const yn = isNumericIdent(y);
    if (xn && yn) return BigInt(x) < BigInt(y) ? -1 : 1;
    if (xn !== yn) return xn ? -1 : 1;
    return x < y ? -1 : 1;
  }
  return a.length === b.length ? 0 : a.length < b.length ? -1 : 1;
}

/** Must stay in sync with src/contracts/versionorder/cases.yaml, which Go, the CLI and a test here all run. */
export function compareVersions(a: string, b: string): -1 | 0 | 1 {
  const pa = parseVersion(a);
  const pb = parseVersion(b);
  for (let i = 0; i < 3; i++) {
    if (pa.core[i]! !== pb.core[i]!) return pa.core[i]! < pb.core[i]! ? -1 : 1;
  }
  const pre = comparePre(pa.pre, pb.pre);
  return pre < 0 ? -1 : pre > 0 ? 1 : 0;
}

/** A prerelease carries a `-` suffix; a `+` suffix is build metadata and never one. */
export const isPrerelease = (v: string | undefined): boolean => !!v && parseVersion(v).pre.length > 0;
