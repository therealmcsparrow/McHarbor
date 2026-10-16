// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Find t() calls whose dot-prefixed key is not present in the bound
// useTranslation namespace but IS present in another namespace (most
// commonly common). These resolve to the literal key string at runtime
// and look like "nav.containers", "plugin.confirm", etc.

import console from 'node:console';
import { readFileSync, readdirSync } from 'node:fs';
import { join, resolve, relative } from 'node:path';
import { execSync } from 'node:child_process';

const repoRoot = resolve('.');
const srcRoot = resolve('src/frontend');
const localeDir = join(srcRoot, 'core/i18n/locales/en');

const grepTargets = [
  join(srcRoot, 'modules'),
  join(srcRoot, 'resources'),
  join(srcRoot, 'core'),
  join(repoRoot, 'widgets'),
  join(repoRoot, 'nodes'),
];

const fileList = [];
for (const target of grepTargets) {
  try {
    const out = execSync(`grep -rl "useTranslation(" --include="*.ts" --include="*.tsx" "${target}"`, {
      encoding: 'utf8',
    }).trim();
    if (out) fileList.push(...out.split('\n').filter(Boolean));
  } catch {
    /* target dir may not exist */
  }
}

const SKIP = ['node_modules', 'dist', 'build', 'coverage', '.git'];
const filtered = fileList.filter((f) => !SKIP.some((s) => f.includes(`/${s}/`)));

// Load all en locale files
const locales = {};
for (const f of readdirSync(localeDir)) {
  if (!f.endsWith('.json')) continue;
  const raw = readFileSync(join(localeDir, f), 'utf8');
  locales[f.replace('.json', '')] = JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function get(obj, key) {
  return key.split('.').reduce((o, k) => (o == null ? undefined : o[k]), obj);
}

const findings = [];
for (const file of filtered) {
  let text;
  try {
    text = readFileSync(file, 'utf8');
  } catch {
    continue;
  }

  // Strip block + line comments
  text = text.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '));
  text = text.replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));

  const nsList = [...text.matchAll(/useTranslation\(\s*['"]([^'"]+)['"]\s*\)/g)].map((m) => m[1]);

  // Find t( calls with static dot-prefixed keys
  const tRegex = /\bt\(\s*['"]([a-z][a-zA-Z0-9]*\.[a-zA-Z0-9_.]+)['"]/g;
  let m;
  while ((m = tRegex.exec(text)) !== null) {
    const key = m[1];
    const line = text.slice(0, m.index).split('\n').length;
    // For each bound namespace, check if the key is present there
    for (const ns of nsList) {
      if (get(locales[ns], key)) continue; // OK — present in bound ns
      // Look for it in any other namespace
      const found = [];
      for (const [otherNs, tree] of Object.entries(locales)) {
        if (otherNs === ns) continue;
        if (get(tree, key)) found.push(otherNs);
      }
      if (found.length > 0) {
        findings.push({ file: relative(repoRoot, file), line, ns, key, in: found });
      } else {
        // not present anywhere — also worth flagging
        findings.push({ file: relative(repoRoot, file), line, ns, key, in: [], missing: true });
      }
    }
  }
}

const missing = findings.filter((f) => f.missing);
const mismatched = findings.filter((f) => !f.missing);

console.log(`Namespace mismatches: ${mismatched.length}`);
mismatched.slice(0, 60).forEach((f) => {
  console.log(`  ${f.file}:${f.line}  ns=${f.ns}  t('${f.key}')  → in ${f.in.join(', ')}`);
});
if (mismatched.length > 60) console.log(`  ... +${mismatched.length - 60} more`);

console.log(`\nMissing keys (key not in any locale): ${missing.length}`);
missing.slice(0, 30).forEach((f) => {
  console.log(`  ${f.file}:${f.line}  ns=${f.ns}  t('${f.key}')`);
});
if (missing.length > 30) console.log(`  ... +${missing.length - 30} more`);
