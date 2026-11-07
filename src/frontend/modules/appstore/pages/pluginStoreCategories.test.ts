// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { compareVersions, isPrerelease } from '@resources/utils/versionOrder';
import { isNewerArtifact, isNewerVersion } from './pluginStoreCategories';

type Row = Record<string, string | number | boolean>;

// cases.yaml holds one flow-mapping per line; this reads exactly that shape so the
// frontend runs the same table as Go and the CLI without a YAML dependency.
function rows(section: string): Row[] {
  const text = readFileSync(resolve(__dirname, '../../../../contracts/versionorder/cases.yaml'), 'utf8');
  const lines = text.split('\n');
  const start = lines.findIndex((l) => l.trim() === `${section}:`);
  const out: Row[] = [];
  for (const line of lines.slice(start + 1)) {
    const m = /^\s+- \{ (.*) \}\s*$/.exec(line);
    if (!m) break;
    const row: Row = {};
    for (const kv of m[1]!.matchAll(/(\w+): ("(?:[^"\\]|\\.)*"|-?\d+|true|false)/g)) {
      row[kv[1]!] = JSON.parse(kv[2]!) as string | number | boolean;
    }
    out.push(row);
  }
  return out;
}

describe('version order shared table', () => {
  const compare = rows('compare');
  const prerelease = rows('prerelease');

  it('reads every row of the table', () => {
    expect(compare.length).toBeGreaterThan(15);
    expect(prerelease.length).toBeGreaterThan(5);
  });

  it.each(compare)('compare $a vs $b is $want', (row) => {
    expect(compareVersions(row.a as string, row.b as string)).toBe(row.want);
  });

  it.each(prerelease)('prerelease $input is $want', (row) => {
    expect(isPrerelease(row.input as string)).toBe(row.want);
  });

  it('isNewerVersion follows the order, so a prerelease is not newer than its final', () => {
    expect(isNewerVersion('2026.11.02-rc.1', '2026.11.02')).toBe(false);
    expect(isNewerVersion('2026.11.02', '2026.11.02-rc.1')).toBe(true);
    expect(isNewerVersion('2026.11.02-rc.1', '2026.11.01')).toBe(true);
  });

  it('isNewerArtifact keeps the build tiebreak at an equal version', () => {
    expect(isNewerArtifact('2026.11.01', '202611060900', '2026.11.01', '202611060100')).toBe(true);
    expect(isNewerArtifact('2026.11.02-rc.1', undefined, '2026.11.02', undefined)).toBe(false);
  });
});
