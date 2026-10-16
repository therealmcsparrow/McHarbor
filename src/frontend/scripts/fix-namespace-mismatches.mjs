// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Auto-fix t() / tc() calls where the bound useTranslation namespace
// does not contain the key but another namespace does. Adds
// `ns: '<other>'` to the call's options bag so i18next looks up the
// key in the correct namespace.
//
// Also flags (without fixing) keys that are missing from every locale.
// Those need human-authored translations before they can render.
//
// Run from src/frontend: `node scripts/fix-namespace-mismatches.mjs`.

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

function findNamespaceForKey(key) {
  // Find which namespace owns the key (longest prefix match first)
  let best = null;
  let bestLen = -1;
  for (const [ns, tree] of Object.entries(locales)) {
    if (get(tree, key)) {
      if (ns.length > bestLen) {
        best = ns;
        bestLen = ns.length;
      }
    }
  }
  return best;
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

  // Strip block + line comments so commented-out calls aren't matched
  const stripped = text
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));

  // Find every t(...) / tc(...) / tFunc(...) call with a static key.
  // Match: (t|tc|i18n\.t)(\s*)?\('key', { ...options })
  // Strategy: scan with a regex, then use paren-balanced scanning to
  // walk the call's argument list and rewrite the options bag.
  const callRegex = /\b(?:t|i18n\.t)\s*\(/g;
  let fixed = 0;
  let m;
  let edited = text;

  // We'll do this in a single pass by re-scanning after each edit.
  let scanText = stripped;

  // First, collect every call with its surrounding options text.
  while ((m = callRegex.exec(scanText)) !== null) {
    const openIdx = m.index + m[0].length - 1; // index of '('
    // Find matching close
    let depth = 1;
    let i = openIdx + 1;
    let quote = null;
    while (i < scanText.length && depth > 0) {
      const c = scanText[i];
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
    if (depth !== 0) continue; // malformed
    const closeIdx = i - 1;

    const args = scanText.slice(openIdx + 1, closeIdx);
    // Find top-level comma
    let depth2 = 0;
    let quote2 = null;
    let commaIdx = -1;
    for (let j = 0; j < args.length; j += 1) {
      const c = args[j];
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
    if (commaIdx === -1) {
      // t('key') with no options — we still want to add `, { ns: '...' }`
      const keyPart = args.trim();
      const km2 = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
      if (!km2) continue;
      const key = km2[1];
      if (key.includes('{') || key.includes('$')) continue;
      const before2 = scanText.slice(0, m.index);
      const allBindings2 = [...before2.matchAll(/useTranslation\(\s*([^)]+)\)/g)];
      if (allBindings2.length === 0) continue;
      const lastBinding2 = allBindings2[allBindings2.length - 1];
      const bindingArgs2 = lastBinding2[1];
      const boundNs2 = [];
      for (const nm of bindingArgs2.matchAll(/['"]([^'"]+)['"]/g)) boundNs2.push(nm[1]);
      const ok2 = boundNs2.some((ns) => get(locales[ns], key));
      if (ok2) continue;
      const owner2 = findNamespaceForKey(key);
      if (!owner2) {
        missingKeys.add(key);
        continue;
      }
      if (boundNs2.includes(owner2)) continue;
      // Insert `, { ns: '<owner>' }` at closeIdx (before the `)`)
      const ins = `, { ns: '${owner2}' }`;
      edited = edited.slice(0, closeIdx) + ins + edited.slice(closeIdx);
      fixed += 1;
      continue;
    }
    const keyPart = args.slice(0, commaIdx).trim();

    const km = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
    if (!km) continue;
    const key = km[1];
    if (key.includes('{') || key.includes('$')) continue; // dynamic

    // Determine bound namespace(s) — search for the nearest enclosing
    // useTranslation binding. This is approximate (we don't fully
    // parse scope), but matches the project's convention that
    // useTranslation is at component top, t() inside JSX.
    const before = scanText.slice(0, m.index);
    const allBindings = [...before.matchAll(/useTranslation\(\s*([^)]+)\)/g)];
    if (allBindings.length === 0) continue;
    const lastBinding = allBindings[allBindings.length - 1];
    const bindingArgs = lastBinding[1];
    const boundNs = [];
    for (const nm of bindingArgs.matchAll(/['"]([^'"]+)['"]/g)) boundNs.push(nm[1]);

    // Check if any bound namespace has the key
    const ok = boundNs.some((ns) => get(locales[ns], key));
    if (ok) continue;

    // Find which namespace actually owns it
    const owner = findNamespaceForKey(key);
    if (!owner) {
      missingKeys.add(key);
      continue;
    }
    if (boundNs.includes(owner)) continue; // shouldn't happen given ok check

    // We need to add ns: '<owner>' to the options bag.
    // The options part starts at `{`. If it's an empty `{}`, replace
    // with `{ ns: '<owner>' }`. Otherwise add `ns: '<owner>', ` at
    // the start of the bag (after the opening brace).
    // Build the replacement string for the OPTIONAL second argument.
    // We have the original `args` and need to insert `ns: '<owner>'`
    // either before existing keys or as a fresh object.

    // Identify the start of the options bag in the original (un-stripped)
    // text. The stripped text preserves positions, so the commaIdx in
    // args maps to the original.
    // Better: find the offset in the full file.
    // The call's opening paren is at openIdx; commaIdx is relative to
    // args (which starts at openIdx+1).
    const optsStartInFile = openIdx + 1 + commaIdx + 1; // after the comma
    // Skip whitespace in the original edited text
    let oi = optsStartInFile;
    while (oi < edited.length && /\s/.test(edited[oi])) oi += 1;
    const firstBrace = edited[oi];

    let replacement;
    if (firstBrace === '{') {
      // Check if bag is empty {}
      if (edited[oi + 1] === '}') {
        replacement = `{ ns: '${owner}' }`;
      } else {
        // Insert `ns: '...', ` after the opening brace
        replacement = `{ ns: '${owner}', `;
      }
    } else {
      // No options object — need to add one
      replacement = `, { ns: '${owner}' }`;
    }

    // Replace in edited text starting at the position where the
    // options should be. We have to be careful: if firstBrace === '{',
    // we insert at oi+1 (after the brace) or replace `{}`. Otherwise
    // we insert at optsStartInFile (where the comma was).
    if (firstBrace === '{') {
      if (edited[oi + 1] === '}') {
        // Replace `{}` (positions oi to oi+1 inclusive)
        edited = edited.slice(0, oi) + replacement + edited.slice(oi + 2);
      } else {
        // Insert after `{` (position oi + 1)
        edited = edited.slice(0, oi + 1) + replacement + edited.slice(oi + 1);
      }
    } else {
      edited = edited.slice(0, optsStartInFile) + replacement + edited.slice(optsStartInFile);
    }
    fixed += 1;
  }

  if (fixed > 0) {
    writeFileSync(file, edited, 'utf8');
    perFile.push({ file: relative(repoRoot, file), fixed });
    totalFixed += fixed;
  }
}

console.log(`fix-namespace-mismatches: added ${totalFixed} 'ns:' option(s)`);
perFile.slice(0, 30).forEach(({ file, fixed }) => console.log(`  ${file}: ${fixed}`));
if (perFile.length > 30) console.log(`  ... +${perFile.length - 30} more files`);
console.log(`\nMissing keys (no locale has them — need author): ${missingKeys.size}`);
[...missingKeys].slice(0, 50).forEach((k) => console.log(`  - ${k}`));
if (missingKeys.size > 50) console.log(`  ... +${missingKeys.size - 50} more`);
