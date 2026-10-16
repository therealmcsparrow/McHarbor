// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * check-referenced-keys — find i18n keys the code *uses* but no locale defines.
 *
 * `check-i18n.mjs` compares locales against each other: it catches a key
 * present in `en` and missing in `de`. It cannot catch a key missing from
 * every locale, because there is nothing to compare against.
 *
 * That is the gap that let `nav.section.operate` ship. The sidebar sections
 * were renamed operate / deliver / govern, the labelKeys were updated, and
 * the strings were never written — so the UI rendered the raw key,
 * `NAV.SECTION.OPERATE`, in the sidebar of every install.
 *
 * This scans for literal key references and asserts each resolves.
 *
 * Usage: node scripts/check-referenced-keys.mjs
 */

import console from 'node:console';
import process from 'node:process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(HERE, '..');
const LOCALES = path.join(ROOT, 'core', 'i18n', 'locales', 'en');

/** Flatten one namespace to a Set of dotted keys. */
function flatten(value, prefix, out) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    out.add(prefix);
    return out;
  }
  for (const [k, v] of Object.entries(value)) {
    flatten(v, prefix ? `${prefix}.${k}` : k, out);
  }
  return out;
}

const namespaces = {};
for (const file of fs.readdirSync(LOCALES).filter((f) => f.endsWith('.json'))) {
  const ns = file.replace(/\.json$/, '');
  namespaces[ns] = flatten(JSON.parse(fs.readFileSync(path.join(LOCALES, file), 'utf8')), '', new Set());
}

// Widget/node translations are co-located and merged into `dashboard`
// and `nodes` at runtime, so fold them in before checking.
for (const [dir, ns] of [
  [path.join(ROOT, '..', '..', 'widgets'), 'dashboard'],
  [path.join(ROOT, '..', '..', 'nodes'), 'nodes'],
]) {
  if (!fs.existsSync(dir)) continue;
  for (const entry of fs.readdirSync(dir)) {
    const en = path.join(dir, entry, 'i18n', 'en.json');
    if (!fs.existsSync(en)) continue;
    flatten(JSON.parse(fs.readFileSync(en, 'utf8')), '', namespaces[ns]);
  }
}

// `labelKey: 'nav.section.operate'` and similar static key fields. These are
// the dangerous ones: a template-built key (`detail.${tab}`) can't be checked
// statically, but a literal one always can.
const FIELD_RE = /\b(?:labelKey|descriptionKey|titleKey|nameKey|groupLabelKey)\s*:\s*['"]([a-zA-Z][\w.]*)['"]/g;

const problems = [];

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (['node_modules', 'dist', 'build', 'coverage'].includes(entry.name)) continue;
      walk(p);
      continue;
    }
    if (!/\.(ts|tsx)$/.test(entry.name)) continue;
    const src = fs.readFileSync(p, 'utf8');

    for (const m of src.matchAll(FIELD_RE)) {
      const raw = m[1];
      // `ns:key` form, or bare key resolved against a default namespace.
      const [ns, key] = raw.includes(':') ? raw.split(':') : [null, raw];
      const rel = path.relative(ROOT, p).replace(/\\/g, '/');

      if (ns) {
        if (namespaces[ns] && !namespaces[ns].has(key)) {
          problems.push(`${rel}: ${raw} — not in ${ns}.json`);
        }
        continue;
      }
      // No namespace given: accept it if ANY namespace defines it.
      const found = Object.values(namespaces).some((set) => set.has(key));
      if (!found) problems.push(`${rel}: ${raw} — not in any namespace`);
    }
  }
}

walk(path.join(ROOT, 'modules'));
walk(path.join(ROOT, 'resources'));

for (const p of problems) console.error(`  MISSING  ${p}`);

if (problems.length) {
  console.error(`\n${problems.length} referenced key(s) resolve to nothing — the UI renders the raw key.`);
  process.exit(1);
}
console.log('OK — every statically referenced i18n key resolves.');
