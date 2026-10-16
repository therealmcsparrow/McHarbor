// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const REPO = resolve(__dirname, '..', '..', '..');
const ALLOWLIST_PATH = resolve(__dirname, 'fixtures', 'query-envelope-allowlist.json');
const SCAN_ROOTS = ['core', 'modules', 'resources'].map((dir) => join(REPO, 'src/frontend', dir));
const SKIP_DIRS = new Set(['node_modules', 'dist', 'build', 'coverage']);

type Allowed = { file: string; count: number; reason: string };

// The same shape ESLint's queryFn selector flags: `.then((r) => r.data)`, in
// either parameter-name variation this codebase uses. A queryFn ending here
// throws away everything but `data`, so a failed request never reaches
// isError — this is the vitest backstop for that ESLint rule.
const THEN_R_DATA = /\.then\(\s*\(?\s*(\w+)\s*\)?\s*=>\s*\1\.data\s*\)/g;

function walk(dir: string, out: string[]) {
  if (!existsSync(dir)) return;
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      walk(path, out);
    } else if (/\.(ts|tsx)$/.test(path) && !/\.(test|spec)\.tsx?$/.test(path)) {
      out.push(path);
    }
  }
}

/** skipStringLiteral advances past a '..' or "..." literal starting at i (source[i] is the opening quote). */
function skipStringLiteral(source: string, i: number): number {
  const quote = source[i];
  i++;
  while (i < source.length && source[i] !== quote) {
    i += source[i] === '\\' ? 2 : 1;
  }
  return i + 1;
}

/**
 * skipTemplateLiteral advances past a template literal starting at i
 * (source[i] is the opening backtick), tracking `${...}` nesting depth so a
 * nested template's own backtick never ends the scan early.
 */
function skipTemplateLiteral(source: string, i: number): number {
  i++;
  let depth = 0;
  while (i < source.length) {
    const c = source[i];
    if (c === '\\') {
      i += 2;
      continue;
    }
    if (depth === 0 && c === '`') return i + 1;
    if (depth === 0 && c === '$' && source[i + 1] === '{') {
      depth = 1;
      i += 2;
      continue;
    }
    if (depth > 0) {
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '`') {
        i = skipTemplateLiteral(source, i);
        continue;
      } else if (c === '"' || c === "'") {
        i = skipStringLiteral(source, i);
        continue;
      }
    }
    i++;
  }
  return i;
}

/**
 * stripComments removes every `//` and `/* *\/` comment from source while
 * leaving string and template literals untouched, so a comment that merely
 * *mentions* `.then((r) => r.data)` (documenting the anti-pattern, as
 * `useMFA.ts` does) is never mistaken for the pattern itself.
 */
function stripComments(source: string): string {
  let out = '';
  let i = 0;
  while (i < source.length) {
    const c = source[i];
    const c2 = source[i + 1];
    if (c === '/' && c2 === '/') {
      while (i < source.length && source[i] !== '\n') i++;
      continue;
    }
    if (c === '/' && c2 === '*') {
      i += 2;
      while (i < source.length && !(source[i] === '*' && source[i + 1] === '/')) i++;
      i += 2;
      continue;
    }
    if (c === '"' || c === "'") {
      const end = skipStringLiteral(source, i);
      out += source.slice(i, end);
      i = end;
      continue;
    }
    if (c === '`') {
      const end = skipTemplateLiteral(source, i);
      out += source.slice(i, end);
      i = end;
      continue;
    }
    out += c;
    i++;
  }
  return out;
}

/**
 * queryFnBodies returns the balanced value text following every `queryFn:`
 * key in source — the value of the property, from just after the colon up to
 * the comma or brace that closes it at the same nesting depth. Scoping to
 * this value (rather than the whole file) is what keeps a `mutationFn:`'s own
 * `.then((r) => r.data)`, like ExecTab.tsx's, out of this check: that shape is
 * real too, but it is onError/onSuccess-driven rather than isError-driven, and
 * ESLint's selector — the rule this test backs up — only targets `queryFn`.
 */
function queryFnBodies(source: string): string[] {
  const bodies: string[] = [];
  const keyRe = /\bqueryFn\s*:/g;
  for (const match of source.matchAll(keyRe)) {
    let i = match.index! + match[0].length;
    const start = i;
    let depth = 0;
    while (i < source.length) {
      const c = source[i];
      if (depth === 0 && (c === ',' || c === '}')) break;
      if (c === '"' || c === "'") {
        i = skipStringLiteral(source, i);
        continue;
      }
      if (c === '`') {
        i = skipTemplateLiteral(source, i);
        continue;
      }
      if (c === '(' || c === '{' || c === '[') depth++;
      else if (c === ')' || c === '}' || c === ']') depth--;
      i++;
    }
    bodies.push(source.slice(start, i));
  }
  return bodies;
}

function countHits(source: string): number {
  const cleaned = stripComments(source);
  let count = 0;
  for (const body of queryFnBodies(cleaned)) {
    THEN_R_DATA.lastIndex = 0;
    count += (body.match(THEN_R_DATA) ?? []).length;
  }
  return count;
}

function scanHitsByFile(): Map<string, number> {
  const files: string[] = [];
  for (const root of SCAN_ROOTS) walk(root, files);

  const hits = new Map<string, number>();
  for (const file of files) {
    const rel = relative(REPO, file).replace(/\\/g, '/');
    const count = countHits(readFileSync(file, 'utf8'));
    if (count > 0) hits.set(rel, count);
  }
  return hits;
}

const allowList = JSON.parse(readFileSync(ALLOWLIST_PATH, 'utf8')) as Allowed[];

describe('queryFn does not collapse to .then((r) => r.data)', () => {
  const hits = scanHitsByFile();
  const allowedByFile = new Map(allowList.map((entry) => [entry.file, entry]));

  it('has no queryFn envelope collapse outside the allow-list, and no allow-listed file exceeds its recorded count', () => {
    const offenders: string[] = [];
    for (const [file, count] of hits) {
      const entry = allowedByFile.get(file);
      if (!entry) {
        offenders.push(`${file}: ${count} (not allow-listed)`);
      } else if (count > entry.count) {
        offenders.push(`${file}: ${count} (allow-listed for ${entry.count})`);
      }
    }
    expect(
      offenders,
      'end the queryFn in .then(assertSuccess) so a failed request reaches isError, or add/raise a justified allow-list entry',
    ).toEqual([]);
  });

  it('keeps every allow-list entry justified, current, and not stale', () => {
    for (const entry of allowList) {
      expect(entry.reason.trim().length, `${entry.file} needs a reason`).toBeGreaterThan(10);
      const actual = hits.get(entry.file) ?? 0;
      expect(
        actual,
        `${entry.file} no longer collapses to .then((r) => r.data) in any queryFn; drop it from the allow-list`,
      ).toBeGreaterThan(0);
      expect(
        actual,
        `${entry.file}'s count dropped to ${actual} (allow-listed for ${entry.count}); lower the allow-list entry to match`,
      ).toBe(entry.count);
    }
  });
});
