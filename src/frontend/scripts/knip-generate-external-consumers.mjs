// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Regenerates knip-external-consumers.ts: one aliased type-only re-export per host name that
// widgets/, nodes/ and src/plugins/Plugins/*/frontend/src import through @resources, @core or
// @modules. Knip only analyses files under src/frontend, so this entry file is how it learns
// those trees use the host, at export precision.
//
// Usage: node scripts/knip-generate-external-consumers.mjs [--check] [--allow-partial]
//                                                          [--plugins-dir <dir>]
//   --check          write nothing; exit 1 when the committed file is stale
//   --allow-partial  proceed when the plugin tree is absent (the output then omits plugin uses)
//   --plugins-dir    override src/plugins/Plugins (used to simulate a checkout without it)

import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const FRONTEND_DIR = path.resolve(import.meta.dirname, '..');
const REPO_ROOT = path.resolve(FRONTEND_DIR, '../..');
const OUT_PATH = path.join(FRONTEND_DIR, 'knip-external-consumers.ts');

const args = process.argv.slice(2);
const check = args.includes('--check');
const allowPartial = args.includes('--allow-partial');
const pluginsFlag = args.indexOf('--plugins-dir');
const PLUGINS_DIR =
  pluginsFlag >= 0 ? path.resolve(args[pluginsFlag + 1]) : path.join(REPO_ROOT, 'src', 'plugins', 'Plugins');

const ALIASES = {
  '@resources': path.join(FRONTEND_DIR, 'resources'),
  '@core': path.join(FRONTEND_DIR, 'core'),
  '@modules': path.join(FRONTEND_DIR, 'modules'),
};

const log = (msg) => globalThis.console.log(msg);

function pluginFrontendDirs() {
  if (!existsSync(PLUGINS_DIR)) return null;
  return readdirSync(PLUGINS_DIR)
    .sort()
    .map((slug) => path.join(PLUGINS_DIR, slug, 'frontend', 'src'))
    .filter((dir) => existsSync(dir));
}

const pluginDirs = pluginFrontendDirs();
if (pluginDirs === null && !allowPartial) {
  globalThis.console.error(
    `Plugin tree not found at ${PLUGINS_DIR}; refusing to generate a partial file (pass --allow-partial to override).`,
  );
  process.exit(2);
}

const trees = [path.join(REPO_ROOT, 'widgets'), path.join(REPO_ROOT, 'nodes'), ...(pluginDirs ?? [])];

function walk(dir) {
  const out = [];
  const stack = [dir];
  while (stack.length > 0) {
    const current = stack.pop();
    if (!existsSync(current)) continue;
    for (const entry of readdirSync(current)) {
      if (entry === 'node_modules' || entry === 'dist') continue;
      const full = path.join(current, entry);
      if (statSync(full).isDirectory()) stack.push(full);
      else if (/\.(ts|tsx)$/.test(entry) && !entry.endsWith('.d.ts')) out.push(full);
    }
  }
  return out;
}

const IMPORT_RE =
  /\b(?:import|export)\s+(?:type\s+)?([^;'"]*?)\s+from\s+['"]([^'"]+)['"]|\bimport\s+['"]([^'"]+)['"]|\bimport\(\s*['"]([^'"]+)['"]\s*\)/g;

function namesOf(clause) {
  if (clause === undefined) return new Set(['*']);
  if (/\*\s+as\s+\w+/.test(clause)) return new Set(['*']);
  const names = new Set();
  const brace = clause.match(/\{([^}]*)\}/);
  if (brace) {
    for (const part of brace[1].split(',')) {
      const original = part
        .replace(/^\s*type\s+/, '')
        .trim()
        .split(/\s+as\s+/)[0]
        .trim();
      if (original) names.add(original);
    }
  }
  if (clause.split('{')[0].replace(/,$/, '').trim()) names.add('default');
  return names;
}

function resolveToFile(base) {
  for (const candidate of [
    `${base}.ts`,
    `${base}.tsx`,
    path.join(base, 'index.ts'),
    path.join(base, 'index.tsx'),
    base,
  ]) {
    if (/\.(ts|tsx)$/.test(candidate) && existsSync(candidate) && statSync(candidate).isFile()) {
      return path
        .relative(FRONTEND_DIR, candidate)
        .replace(/\\/g, '/')
        .replace(/\.(ts|tsx)$/, '');
    }
  }
  return null;
}

function resolveAlias(specifier) {
  for (const [alias, dir] of Object.entries(ALIASES)) {
    if (specifier === alias) return resolveToFile(dir);
    if (specifier.startsWith(`${alias}/`)) return resolveToFile(path.join(dir, specifier.slice(alias.length + 1)));
  }
  return null;
}

const used = new Map();
let consumerFiles = 0;
for (const tree of trees) {
  for (const file of walk(tree)) {
    consumerFiles += 1;
    const content = readFileSync(file, 'utf8');
    IMPORT_RE.lastIndex = 0;
    let m;
    while ((m = IMPORT_RE.exec(content))) {
      const [, clause, spec, sideEffect, dynamic] = m;
      const resolved = resolveAlias(spec ?? sideEffect ?? dynamic);
      if (!resolved) continue;
      if (!used.has(resolved)) used.set(resolved, new Set());
      const names = dynamic ? new Set(['*']) : namesOf(clause);
      for (const n of names) used.get(resolved).add(n);
    }
  }
}

const lines = [
  '// Copyright (c) 2026 McSparrow. All rights reserved.',
  '// McHarbor is licensed under the McHarbor License. See LICENSE for details.',
  '',
  '// Generated by scripts/knip-generate-external-consumers.mjs (npm run knip:consumers); do not edit.',
  '// Knip entry: records the host exports that widgets/, nodes/ and plugin frontends import.',
  '',
];
/** Matches the default sort order so the generated file stays byte-stable. */
function compareCodeUnits(a, b) {
  if (a < b) return -1;
  return a > b ? 1 : 0;
}
let index = 0;
for (const file of [...used.keys()].sort(compareCodeUnits)) {
  index += 1;
  const names = [...used.get(file)].sort(compareCodeUnits);
  const spec = `./${file}`;
  if (names.includes('*')) {
    lines.push(`export type * as m${index} from '${spec}';`);
    continue;
  }
  const bindings = names.map((n) => `${n} as m${index}_${n}`).join(', ');
  lines.push(`export type { ${bindings} } from '${spec}';`);
}
const output = `${lines.join('\n')}\n`;

if (check) {
  const current = existsSync(OUT_PATH) ? readFileSync(OUT_PATH, 'utf8') : '';
  if (current !== output) {
    globalThis.console.error('knip-external-consumers.ts is stale; run npm run knip:consumers');
    process.exit(1);
  }
  log('knip-external-consumers.ts is up to date');
} else {
  writeFileSync(OUT_PATH, output);
  log(`Scanned ${consumerFiles} consumer files; wrote ${used.size} host modules to knip-external-consumers.ts`);
}
