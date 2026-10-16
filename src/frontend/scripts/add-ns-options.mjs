// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Auto-add `ns: '<owner>'` to t() calls whose static key resolves in a
// namespace other than the one bound by useTranslation. Idempotent.
//
// Run from src/frontend: `node scripts/add-ns-options.mjs`.

import console from 'node:console';
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { join, resolve, relative } from 'node:path';
import { execSync } from 'node:child_process';

const frontendRoot = resolve('.');
const repoRoot = resolve('..', '..');
const localeDir = join(frontendRoot, 'core/i18n/locales/en');

const grepTargets = [
  join(frontendRoot, 'modules'),
  join(frontendRoot, 'resources'),
  join(frontendRoot, 'core'),
  join(repoRoot, 'widgets'),
  join(repoRoot, 'nodes'),
];

const fileList = new Set();
for (const target of grepTargets) {
  try {
    const out = execSync(`grep -rl "useTranslation(" --include="*.ts" --include="*.tsx" "${target}"`, {
      encoding: 'utf8',
    }).trim();
    if (out)
      out
        .split('\n')
        .filter(Boolean)
        .forEach((f) => fileList.add(f));
  } catch {
    /* skip */
  }
}

const SKIP = ['node_modules', 'dist', 'build', 'coverage', '.git'];
const files = [...fileList].filter((f) => !SKIP.some((s) => f.includes(`/${s}/`)));

const locales = {};
for (const f of readdirSync(localeDir)) {
  if (!f.endsWith('.json')) continue;
  const raw = readFileSync(join(localeDir, f), 'utf8');
  locales[f.replace('.json', '')] = JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function get(obj, key) {
  return key.split('.').reduce((o, k) => (o == null ? undefined : o[k]), obj);
}

function findOwner(key) {
  let best = null;
  let bestLen = -1;
  for (const [ns, tree] of Object.entries(locales)) {
    if (get(tree, key)) {
      // Prefer shorter namespace names so 'common' wins over 'containers'
      // when both happen to define the key (rare; either is acceptable).
      if (best === null || ns.length < bestLen) {
        best = ns;
        bestLen = ns.length;
      }
    }
  }
  return best;
}

// Find the nearest preceding useTranslation binding's namespace(s).
// Match all useTranslation('a') or useTranslation(['a', 'b']) or
// useTranslation("a", "b") calls in the entire pre-text.
function findBoundNses(textBeforeCall) {
  const re = /useTranslation\s*\(\s*([^)]*)\)/g;
  let m;
  let last = null;
  while ((m = re.exec(textBeforeCall)) !== null) {
    const args = m[1];
    // Parse argument list to get string literals.
    const ns = [];
    let i = 0;
    while (i < args.length) {
      const c = args[i];
      if (c === '"' || c === "'") {
        const quote = c;
        let j = i + 1;
        while (j < args.length && args[j] !== quote) j += 1;
        ns.push(args.slice(i + 1, j));
        i = j + 1;
      } else i += 1;
    }
    last = ns;
  }
  return last || [];
}

let totalFixed = 0;
const perFile = [];
const missingKeys = new Set();

for (const file of files) {
  let text;
  try {
    text = readFileSync(file, 'utf8');
  } catch {
    continue;
  }

  // Strip block comments so we don't match commented-out calls.
  const stripped = text
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));

  // Find every t('key', ...) call. We do it by scanning with a regex
  // then walking paren-balanced.
  const callRe = /(?<![.\w])(?:t|i18n\.t)\s*\(/g;
  let m;
  let edits = [];
  while ((m = callRe.exec(stripped)) !== null) {
    const callStart = m.index; // start of `t`
    const openIdx = m.index + m[0].length - 1; // '('
    // Walk to matching close paren
    let depth = 1;
    let i = openIdx + 1;
    let quote = null;
    while (i < stripped.length && depth > 0) {
      const c = stripped[i];
      if (c === '\\') {
        i += 2;
        continue;
      }
      if (quote) {
        if (c === quote) quote = null;
        i += 1;
        continue;
      }
      if (c === '"' || c === "'" || c === '`') {
        quote = c;
        i += 1;
        continue;
      }
      if (c === '(' || c === '{' || c === '[') depth += 1;
      else if (c === ')' || c === '}' || c === ']') depth -= 1;
      i += 1;
    }
    if (depth !== 0) continue;
    const closeIdx = i - 1; // index of ')'

    // Find top-level comma in args
    let depth2 = 0;
    let quote2 = null;
    let commaIdx = -1;
    for (let j = openIdx + 1; j < closeIdx; j += 1) {
      const c = stripped[j];
      if (c === '\\') {
        j += 1;
        continue;
      }
      if (quote2) {
        if (c === quote2) quote2 = null;
        continue;
      }
      if (c === '"' || c === "'" || c === '`') {
        quote2 = c;
        continue;
      }
      if (c === '(' || c === '{' || c === '[') depth2 += 1;
      else if (c === ')' || c === '}' || c === ']') depth2 -= 1;
      else if (c === ',' && depth2 === 0) {
        commaIdx = j;
        break;
      }
    }

    let key;
    let optsStart = closeIdx;
    let optsEnd = closeIdx;
    if (commaIdx === -1) {
      // t('key') — single arg
      const keyPart = stripped.slice(openIdx + 1, closeIdx).trim();
      const km = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
      if (!km) continue;
      key = km[1];
    } else {
      const keyPart = stripped.slice(openIdx + 1, commaIdx).trim();
      const km = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
      if (!km) continue;
      key = km[1];
      optsStart = commaIdx + 1;
      // Skip leading whitespace to find options bag
      let oi = optsStart;
      while (oi < closeIdx && /\s/.test(stripped[oi])) oi += 1;
      if (stripped[oi] === '{') {
        optsStart = oi;
        depth2 = 1;
        quote2 = null;
        oi += 1;
        while (oi < closeIdx && depth2 > 0) {
          const c = stripped[oi];
          if (c === '\\') {
            oi += 2;
            continue;
          }
          if (quote2) {
            if (c === quote2) quote2 = null;
            oi += 1;
            continue;
          }
          if (c === '"' || c === "'" || c === '`') {
            quote2 = c;
            oi += 1;
            continue;
          }
          if (c === '(' || c === '{' || c === '[') depth2 += 1;
          else if (c === ')' || c === '}' || c === ']') depth2 -= 1;
          oi += 1;
        }
        optsEnd = oi - 1; // index of closing '}'
      }
    }

    if (key.includes('{') || key.includes('$')) continue;

    // Find bound namespaces from preceding useTranslation calls
    const before = stripped.slice(0, callStart);
    const boundNses = findBoundNses(before);
    if (boundNses.length === 0) continue;

    // Does any bound ns have the key?
    const ok = boundNses.some((ns) => get(locales[ns], key));
    if (ok) continue;

    // Find owner
    const owner = findOwner(key);
    if (!owner) {
      missingKeys.add(key);
      continue;
    }
    if (boundNses.includes(owner)) continue;

    // Check if call already has ns: '<owner>' in its options bag
    if (optsStart < optsEnd) {
      const optsText = stripped.slice(optsStart, optsEnd + 1);
      const nsRegex = new RegExp(`\\bns\\s*:\\s*['"]${owner.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}['"]`);
      if (nsRegex.test(optsText)) continue;
      // Also skip if there's any ns: already — manual placement wins
      if (/\bns\s*:/.test(optsText)) continue;
    }

    // Build the insertion. We insert BEFORE the closing paren at closeIdx.
    let ins;
    if (optsStart < optsEnd) {
      // Existing non-empty options bag — insert `, ns: '<owner>'` before
      // the closing brace of the bag.
      ins = `, ns: '${owner}'`;
      edits.push({ pos: optsEnd, len: 0, text: ins });
    } else {
      // No options bag — insert `, { ns: '<owner>' }` before the ')'
      ins = `, { ns: '${owner}' }`;
      edits.push({ pos: closeIdx, len: 0, text: ins });
    }
  }

  if (edits.length === 0) continue;

  // Apply edits in reverse order so positions remain valid
  edits.sort((a, b) => b.pos - a.pos);
  let newText = text;
  for (const e of edits) {
    newText = newText.slice(0, e.pos) + e.text + newText.slice(e.pos);
  }
  writeFileSync(file, newText, 'utf8');
  perFile.push({ file: relative(repoRoot, file), fixed: edits.length });
  totalFixed += edits.length;
}

console.log(`add-ns-options: added ${totalFixed} ns: option(s)`);
perFile.slice(0, 50).forEach(({ file, fixed }) => console.log(`  ${file}: ${fixed}`));
if (perFile.length > 50) console.log(`  ... +${perFile.length - 50} more files`);
console.log(`\nMissing keys (no locale has them): ${missingKeys.size}`);
[...missingKeys].slice(0, 50).forEach((k) => console.log(`  - ${k}`));
if (missingKeys.size > 50) console.log(`  ... +${missingKeys.size - 50} more`);
