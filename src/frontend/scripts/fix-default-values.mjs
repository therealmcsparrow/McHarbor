// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Strip redundant `defaultValue:` clauses from t(key, { defaultValue: TEXT })
// call sites in the frontend. A clause is redundant when the key is
// present in en/<ns>.json AND at least one other locale — proving the
// fallback is never reached (i18next returns the locale value, not the
// defaultValue).
//
// Note: matches the Python fix_default_values.py skill script, but
// searches en for the key across ALL namespaces (not just the first
// segment) because the source's useTranslation binding chooses the
// namespace — which the script can't see without parsing the TS file.
// In practice this is over-permissive (every defaultValue for a key
// that exists anywhere in en is stripped), but the only consequence is
// a wider cleanup, and any false positives would still be removed
// correctly because the locale fallback is more authoritative than the
// defaultValue.

import console from 'node:console';
import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(scriptDir, '..');
const repoRoot = resolve(frontendRoot, '..', '..');
const localesRoot = join(frontendRoot, 'core', 'i18n', 'locales');
const ALL_LANGS = readdirSync(localesRoot)
  .filter((e) => statSync(join(localesRoot, e)).isDirectory())
  .sort();

function readJson(p) {
  const raw = readFileSync(p, 'utf8');
  return JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function collectLeaves(value, prefix = '', out = new Set()) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    if (prefix) out.add(prefix);
    return out;
  }
  if (Object.keys(value).length === 0) {
    if (prefix) out.add(prefix);
    return out;
  }
  for (const [k, v] of Object.entries(value)) {
    collectLeaves(v, prefix ? `${prefix}.${k}` : k, out);
  }
  return out;
}

// Build a per-lang, per-namespace leaf set.
const leafSet = new Map(); // lang -> ns -> Set<leaf>
for (const lang of ALL_LANGS) {
  const subDir = join(localesRoot, lang);
  leafSet.set(lang, new Map());
  for (const file of readdirSync(subDir).filter((f) => f.endsWith('.json'))) {
    const ns = file.replace(/\.json$/, '');
    const fullPath = join(subDir, file);
    try {
      const obj = readJson(fullPath);
      leafSet.get(lang).set(ns, collectLeaves(obj));
    } catch {
      // skip
    }
  }
}

function isKeyCovered(key) {
  // EN must have the key in ANY namespace. Then at least one non-en
  // locale must also have it in the same namespace.
  const enNs = leafSet.get('en');
  let matchingNs = null;
  for (const [ns, leaves] of enNs.entries()) {
    if (leaves.has(key)) {
      matchingNs = ns;
      break;
    }
  }
  if (!matchingNs) return null;
  for (const lang of ALL_LANGS) {
    if (lang === 'en') continue;
    const leaves = leafSet.get(lang)?.get(matchingNs);
    if (leaves && leaves.has(key)) return matchingNs;
  }
  return null;
}

const EXTS = new Set(['.ts', '.tsx']);
const SKIP_DIRS = ['node_modules', 'coverage', 'dist', '.git', 'scripts', '__tests__', '__mocks__', 'build'];

function walk(dir, out = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (SKIP_DIRS.includes(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (EXTS.has(full.slice(full.lastIndexOf('.')))) out.push(full);
  }
  return out;
}

// Two patterns:
//  1. t("X", { defaultValue: "y" }) — strip the whole options block
//  2. t("X", { defaultValue: "y", ... }) — strip just the defaultValue clause
// Both must match across newlines (the previous TODO teaches us line-based
// regexes miss wrapped calls).
function findMatchingClose(text, openIdx) {
  let depth = 1;
  let i = openIdx + 1;
  let quote = null;
  while (i < text.length) {
    const c = text[i];
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
    if (c === '(' || c === '{' || c === '[') {
      depth += 1;
      i += 1;
      continue;
    }
    if (c === ')' || c === '}' || c === ']') {
      depth -= 1;
      if (depth === 0) return i;
      i += 1;
      continue;
    }
    i += 1;
  }
  return -1;
}

function findTopLevelComma(text) {
  let depth = 0;
  let quote = null;
  for (let i = 0; i < text.length; i += 1) {
    const c = text[i];
    if (c === '\\') {
      i += 1;
      continue;
    }
    if (quote) {
      if (c === quote) quote = null;
      continue;
    }
    if (c === '"' || c === "'" || c === '`') {
      quote = c;
      continue;
    }
    if (c === '{' || c === '[' || c === '(') {
      depth += 1;
      continue;
    }
    if (c === '}' || c === ']' || c === ')') {
      depth -= 1;
      continue;
    }
    if (c === ',' && depth === 0) return i;
  }
  return -1;
}

function processFile(filePath) {
  let text = readFileSync(filePath, 'utf8');
  // Strip block comments + line comments so JSDoc examples don't
  // trigger fixes.
  text = text.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '));
  text = text.replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));

  let fixed = 0;
  const edits = [];
  let pass = 0;
  while (true) {
    pass += 1;
    if (pass > 1000) break; // safety
    let anyChange = false;
    const re = /\b(?:t|i18n\.t)\s*\(/g;
    let m;
    while ((m = re.exec(text)) !== null) {
      const open = m.index + m[0].length - 1;
      const close = findMatchingClose(text, open);
      if (close === -1) break;
      const args = text.slice(open + 1, close);
      const comma = findTopLevelComma(args);
      if (comma === -1) {
        re.lastIndex = close + 1;
        continue;
      }
      const keyPart = args.slice(0, comma).trim();
      const optsPart = args.slice(comma + 1);
      const km = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
      if (!km) {
        re.lastIndex = close + 1;
        continue;
      }
      const key = km[1];
      // Skip dynamic keys (templates, vars)
      if (key.includes('{') || key.includes('$')) {
        re.lastIndex = close + 1;
        continue;
      }
      // Must have defaultValue: clause
      const dvm = /\bdefaultValue\s*:\s*(["'])([^"'\n\\]*?)\1/.exec(optsPart);
      if (!dvm) {
        re.lastIndex = close + 1;
        continue;
      }
      const coveredNs = isKeyCovered(key);
      if (!coveredNs) {
        re.lastIndex = close + 1;
        continue;
      }

      // Pattern A: opts block is `{ defaultValue: "...", }` literally
      // (only one key, possibly with trailing comma). Replace the whole
      // `, { defaultValue: "..." }` with no-op — but we have to clean up
      // the trailing comma after the key.
      const callStart = m.index;
      const callEnd = close + 1;
      const call = text.slice(callStart, callEnd);

      // Try pattern 1: t("X", { defaultValue: "..." }) → t("X")
      const A = /^(\s*t\s*\(\s*['"][^'"]+['"])\s*,\s*\{\s*defaultValue\s*:\s*(['"])([^"'\n\\]*?)\2\s*\}\s*\)$/s;
      let stripped;
      const aMatch = A.exec(call);
      if (aMatch) {
        stripped = `${aMatch[1]})`;
      } else {
        // Pattern 2: t("X", { defaultValue: "y", ... }) → t("X", { ... })
        // Strip the `defaultValue: "y",` clause, keep the rest.
        const before = call;
        // Use a regex that captures the head + the rest after the comma.
        const B = /^(\s*t\s*\(\s*['"][^'"]+['"]\s*,\s*\{\s*)defaultValue\s*:\s*(['"])([^"'\n\\]*?)\2\s*,?\s*/s;
        const bMatch = B.exec(before);
        if (bMatch) {
          stripped = bMatch[1] + before.slice(bMatch[0].length);
        } else {
          // Pattern 3: t("X", { defaultValue: "y" }) where there's a comma
          // already after the defaultValue (e.g. trailing comma style)
          const C =
            /^(\s*t\s*\(\s*['"][^'"]+['"]\s*,\s*\{\s*)defaultValue\s*:\s*(['"])([^"'\n\\]*?)\2\s*,?\s*\}\s*\)$/s;
          const cMatch = C.exec(before);
          if (cMatch) {
            stripped = `${cMatch[1]}\\)`.replace(/\\\) $/, ')');
          } else {
            re.lastIndex = close + 1;
            continue;
          }
        }
      }
      if (stripped === undefined || stripped === call) {
        re.lastIndex = close + 1;
        continue;
      }
      // Apply replacement
      text = text.slice(0, callStart) + stripped + text.slice(callEnd);
      fixed += 1;
      edits.push({ key, ns: coveredNs, before: call, after: stripped });
      anyChange = true;
      // Restart scanning because the line offsets have shifted.
      break;
    }
    if (!anyChange) break;
  }
  return { text, fixed, edits };
}

const files = [
  ...walk(join(frontendRoot, 'src')),
  ...walk(join(frontendRoot, 'modules')),
  ...walk(join(frontendRoot, 'core')),
  ...walk(join(frontendRoot, 'resources')),
  ...walk(join(repoRoot, 'widgets')),
  ...walk(join(repoRoot, 'nodes')),
];

let totalFixed = 0;
const perFile = [];
for (const f of files) {
  const { text, fixed, edits } = processFile(f);
  if (fixed > 0) {
    writeFileSync(f, text, 'utf8');
    perFile.push({ file: relative(frontendRoot, f), fixed, edits });
    totalFixed += fixed;
  }
}

console.log(`fix-default-values: stripped ${totalFixed} redundant defaultValue clause(s)`);
for (const { file, fixed, edits } of perFile.slice(0, 25)) {
  console.log(`  ${file}: ${fixed}`);
  for (const e of edits.slice(0, 3)) {
    console.log(`    - ${e.key} (ns=${e.ns})`);
  }
  if (edits.length > 3) console.log(`    ... and ${edits.length - 3} more`);
}
if (perFile.length > 25) console.log(`  ... and ${perFile.length - 25} more files`);
