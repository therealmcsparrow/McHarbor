// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const NODE_BUILTINS = 'fs|path|child_process|os|net|http|https|crypto|stream|zlib|url|util|events|buffer|tls|dns|worker_threads|fs/promises';

// Each rule matches something that is a build-time construct or a Node-only API and
// therefore cannot run when the bundle is loaded as a classic script in the browser.
// `process.env.*` is deliberately absent: healthy bundles carry guarded reads of it.
export const RULES = [
  {
    id: 'unresolved-glob',
    pattern: /\{\}\.glob\(|\bimport\.meta\.glob\b/g,
    why: 'import.meta.glob was not resolved at build time (Vite leaves `{}.glob(` in a UMD build and the call throws)',
  },
  {
    id: 'import-meta',
    pattern: /\bimport\.meta\b/g,
    why: 'import.meta is a syntax error outside an ES module, so the whole bundle fails to parse',
  },
  {
    id: 'node-builtin-require',
    pattern: new RegExp(String.raw`\brequire\(\s*["'\x60](?:node:)?(?:${NODE_BUILTINS})["'\x60]\s*\)`, 'g'),
    why: 'a Node builtin was required; the browser has no such module',
  },
];

/** Returns every finding in `source` as { id, why, offset, excerpt }, ordered by offset. */
export function scan(source) {
  const findings = [];
  for (const rule of RULES) {
    for (const m of source.matchAll(rule.pattern)) {
      const start = Math.max(0, m.index - 30);
      const excerpt = source.slice(start, m.index + m[0].length + 30).replace(/\s+/g, ' ');
      findings.push({ id: rule.id, why: rule.why, offset: m.index, excerpt });
    }
  }
  return findings.sort((a, b) => a.offset - b.offset);
}

/** Checks each file, prints findings to `out`, and returns the process exit code. */
export function check(files, out = console.error) {
  if (files.length === 0) {
    out('usage: check-umd-bundle.mjs <bundle.umd.cjs>...');
    return 2;
  }
  let code = 0;
  for (const file of files) {
    let source;
    try {
      source = readFileSync(file, 'utf8');
    } catch (err) {
      out(`${file}: cannot read (${err.code ?? err.message})`);
      code = Math.max(code, 2);
      continue;
    }
    for (const f of scan(source)) {
      out(`${file}:${f.offset}  ${f.id}  ${f.why}\n    ...${f.excerpt}...`);
      code = Math.max(code, 1);
    }
  }
  return code;
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  process.exit(check(process.argv.slice(2)));
}
