// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Replace `{{TODO: translate}}` placeholder leaves in en/<ns>.json with
// real English text. Two sources:
//
//   1. Harvest `defaultValue: 'TEXT'` arguments from t('key', { defaultValue: ... })
//      calls in src/frontend. These are the source strings the developer
//      already wrote — copying them into en fixes the leaf for free.
//
//   2. For TODO leaves without a defaultValue, derive English from the
//      leaf path. The last segment is camelCase-split ("tabForm" →
//      "Tab Form") and title-cased. Sufficient as a fallback so the
//      UI renders something readable; translators can refine the
//      wording later.
//
// Strategy:
//   - Run the gate first to make sure no `t(...)` call is unresolved.
//   - Walk every en/<ns>.json file, collect leaves whose value is
//     `{{TODO: translate}}`.
//   - Walk src/frontend for `defaultValue: 'literal'` (skip refs),
//     build a key→text map. Last-write wins (modern emitters override
//     earlier ones), which is fine because the audit already showed
//     duplicate keys carry the same English text.
//   - Replace each TODO leaf with the harvested text (preferred) or
//     the derived English (fallback). Leave real strings untouched.
//
// After this runs, run `i18n:backfill-keys` so the new English text
// propagates to the 11 other locales (existing translations preserved
// by setIfMissing — only TODO leaves get overwritten with the new
// English text).

import console from 'node:console';
import { readdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(scriptDir, '..');
const repoRoot = resolve(frontendRoot, '..', '..');
const localesRoot = join(frontendRoot, 'core', 'i18n', 'locales');
const enDir = join(localesRoot, 'en');

const EXTS = new Set(['.ts', '.tsx']);
const SKIP_DIRS = new Set(['node_modules', 'coverage', 'dist', '.git', 'scripts', '__tests__', '__mocks__']);

function isTodo(value) {
  return typeof value === 'string' && /^\s*\{\{\s*TODO\s*:?\s*translate\s*\}\}\s*$/.test(value);
}

function readJson(p) {
  const raw = readFileSync(p, 'utf8');
  return JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function writeJson(p, obj) {
  writeFileSync(p, `${JSON.stringify(obj, null, 2)}\n`, 'utf8');
}

// Walk every leaf in `obj` and call `cb(dottedPath, value)` for each.
function walkLeaves(obj, prefix, cb) {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    if (prefix) cb(prefix, obj);
    return;
  }
  if (Object.keys(obj).length === 0) {
    if (prefix) cb(prefix, obj);
    return;
  }
  for (const [k, v] of Object.entries(obj)) {
    const next = prefix ? `${prefix}.${k}` : k;
    walkLeaves(v, next, cb);
  }
}

function setByPath(obj, dottedPath, value) {
  const parts = dottedPath.split('.');
  let cursor = obj;
  for (let i = 0; i < parts.length - 1; i += 1) {
    const p = parts[i];
    if (cursor[p] == null || typeof cursor[p] !== 'object' || Array.isArray(cursor[p])) {
      cursor[p] = {};
    }
    cursor = cursor[p];
  }
  cursor[parts[parts.length - 1]] = value;
}

// Split a camelCase identifier into Title Case words
// "tabForm" → "Tab Form"
// "advancedHelp" → "Advanced Help"
// "yaml" → "Yaml"
// "isLoading" → "Is Loading"
function humanize(segment) {
  const words = segment
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  return words.map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
}

// Derive an English phrase from a leaf path. Tries to combine the
// leading path segments with the last segment for context — e.g.
// `containers.configEdit.sections.environment` → "Container config:
// Sections Environment" is too long; we just humanize the last
// segment ("Environment") and accept it as a placeholder until a
// translator refines it.
function deriveFromPath(dottedPath) {
  const parts = dottedPath.split('.');
  return humanize(parts[parts.length - 1]);
}

// Walk the frontend for .ts/.tsx files, skipping unwanted dirs.
function walkFrontend(dir, out = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (SKIP_DIRS.has(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walkFrontend(full, out);
    else if (EXTS.has(full.slice(full.lastIndexOf('.')))) out.push(full);
  }
  return out;
}

// Find every t(...) call shape:
//   t('key', { defaultValue: 'TEXT' })
//   t(`key`, { defaultValue: "TEXT" })
//   t('key', { defaultValue: `TEXT` })  ← uncommon but possible
// Walk the source text with paren balancing so wrapping across
// newlines doesn't fool us.
function harvestKeyDefaults(srcRoots) {
  const map = new Map(); // key → english (last write wins)
  // Two-step: first find every `t(<...>` (or `alias(<...>`) opening,
  // then balance parens to find the close, then regex the slice.
  // We don't need exact namespace binding — keys are dotted paths
  // and we look up the leaf by the literal `key` (en uses dot paths).
  const aliasOpen = /\b(?:t|i18n\.t)\s*\(/g;
  for (const root of srcRoots) {
    if (!existsSync(root)) continue;
    for (const file of walkFrontend(root)) {
      let text = readFileSync(file, 'utf8');
      text = text.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '));
      text = text.replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));
      let m;
      while ((m = aliasOpen.exec(text)) !== null) {
        const open = m.index + m[0].length - 1; // the `(` itself
        const close = findMatchingClose(text, open);
        if (close === -1) break;
        const args = text.slice(open + 1, close);
        const comma = findTopLevelComma(args);
        if (comma === -1) {
          aliasOpen.lastIndex = close + 1;
          continue;
        }
        const keyPart = args.slice(0, comma).trim();
        const optsPart = args.slice(comma + 1);
        const km = /^['"`]([^'"`$\\]+)['"`]/.exec(keyPart);
        if (!km) {
          aliasOpen.lastIndex = close + 1;
          continue;
        }
        const key = km[1];
        // Search the options slice for defaultValue: 'literal'.
        const dm = /\bdefaultValue\s*:\s*(["'])([^"'\n\\]*?)\1/.exec(optsPart);
        if (!dm) {
          aliasOpen.lastIndex = close + 1;
          continue;
        }
        const value = dm[2];
        if (value && value.length > 0) map.set(key, value);
        aliasOpen.lastIndex = close + 1;
      }
    }
  }
  return map;
}

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

function main() {
  // The frontend source lives in two places: the bulk is in
  // src/frontend/{modules,resources,core,...} (top-level TSX), and a
  // subset is mirrored under src/frontend/src/frontend/ (a leftover
  // from an earlier layout). Walk both — walking one misses ~95% of
  // the frontstage code.
  const srcRoots = [
    frontendRoot,
    join(frontendRoot, 'src', 'frontend'),
    join(repoRoot, 'widgets'),
    join(repoRoot, 'nodes'),
  ];
  console.log('harvesting defaultValue strings from source...');
  const keyDefaults = harvestKeyDefaults(srcRoots);
  console.log(`  ${keyDefaults.size} key→string mapping(s) harvested`);

  const enFiles = readdirSync(enDir).filter((f) => f.endsWith('.json'));
  let totalReplaced = 0;
  let totalDerived = 0;
  let totalFromDefault = 0;
  const perFile = [];

  for (const file of enFiles) {
    const path = join(enDir, file);
    const obj = readJson(path);
    let replaced = 0;
    let derived = 0;
    let fromDefault = 0;

    walkLeaves(obj, '', (dotted, value) => {
      // Treat both literal `{{TODO: translate}}` strings and empty
      // objects `{}` as leaves to fill. Empty objects are
      // translation-placeholder sentinels — check-i18n.mjs treats
      // them as leaves for parity purposes, so leaving them alone
      // means the non-en locale misses a key its en counterpart
      // (vacuously) carries.
      const isEmptyObject =
        value !== null && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0;
      if (!isTodo(value) && !isEmptyObject) return;
      const harvested = keyDefaults.get(dotted);
      let replacement;
      if (harvested) {
        replacement = harvested;
        fromDefault += 1;
      } else {
        replacement = deriveFromPath(dotted);
        derived += 1;
        // Skip if derivation produced an empty change (e.g. all
        // blank segments). Should never happen for camelCase paths,
        // but guard anyway.
        if (!replacement) replacement = dotted;
      }
      setByPath(obj, dotted, replacement);
      replaced += 1;
    });

    if (replaced > 0) {
      writeJson(path, obj);
      perFile.push(`${file}: -${replaced} (${fromDefault} from defaultValue, ${derived} derived)`);
      totalReplaced += replaced;
      totalDerived += derived;
      totalFromDefault += fromDefault;
    }
  }

  console.log(`Replaced ${totalReplaced} TODO placeholder(s) in en/.`);
  console.log(`  ${totalFromDefault} from t(...) defaultValue(...) arguments`);
  console.log(`  ${totalDerived} derived from leaf path`);
  for (const line of perFile) console.log(`  ${line}`);
}

main();
