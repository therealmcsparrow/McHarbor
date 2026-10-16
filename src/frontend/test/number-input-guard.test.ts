// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const REPO = resolve(__dirname, '..', '..', '..');
const SELF = 'src/frontend/resources/components/ui/NumberInput.tsx';
const SKIP_DIRS = new Set(['node_modules', 'dist', 'build', 'coverage']);
const RAW_NUMBER = [
  /type\s*=\s*["']number["']/,
  /type\s*=\s*\{\s*["'`]number["'`]\s*\}/,
  /type\s*=\s*\{[^}\n]*\?\s*["']number["']/,
];

type Allowed = { file: string; reason: string };

const allowList = JSON.parse(readFileSync(resolve(__dirname, 'number-input-allowlist.json'), 'utf8')) as Allowed[];

function walk(dir: string, out: string[]) {
  if (!existsSync(dir)) return;
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (path.endsWith('.tsx') && !/\.(test|spec)\.tsx$/.test(path)) out.push(path);
  }
}

function scanRoots(): string[] {
  const roots = [
    join(REPO, 'src/frontend/modules'),
    join(REPO, 'src/frontend/resources'),
    join(REPO, 'widgets'),
    join(REPO, 'nodes'),
  ];
  const plugins = join(REPO, 'src/plugins/Plugins');
  if (existsSync(plugins)) {
    for (const slug of readdirSync(plugins)) roots.push(join(plugins, slug, 'frontend', 'src'));
  }
  return roots;
}

function rawNumberInputs(): Map<string, number[]> {
  const files: string[] = [];
  for (const root of scanRoots()) walk(root, files);
  const hits = new Map<string, number[]>();
  for (const path of files) {
    const rel = relative(REPO, path).replace(/\\/g, '/');
    if (rel === SELF) continue;
    const lines = readFileSync(path, 'utf8').split(/\r?\n/);
    lines.forEach((line, index) => {
      if (RAW_NUMBER.some((pattern) => pattern.test(line))) {
        hits.set(rel, [...(hits.get(rel) ?? []), index + 1]);
      }
    });
  }
  return hits;
}

describe('numeric fields use NumberInput', () => {
  const hits = rawNumberInputs();
  const allowed = new Map(allowList.map((entry) => [entry.file, entry.reason]));

  it('renders no raw type="number" input outside the allow-list', () => {
    const offenders = [...hits.entries()]
      .filter(([file]) => !allowed.has(file))
      .map(([file, lines]) => `${file}:${lines.join(',')}`);
    expect(
      offenders,
      'use <NumberInput> from @resources/components/ui/NumberInput, or add a justified allow-list entry',
    ).toEqual([]);
  });

  it('keeps every allow-list entry justified and still needed', () => {
    for (const [file, reason] of allowed) {
      expect(reason.trim().length, `${file} needs a reason`).toBeGreaterThan(10);
      expect(hits.has(file), `${file} no longer renders a raw number input; drop it from the allow-list`).toBe(true);
    }
  });
});
