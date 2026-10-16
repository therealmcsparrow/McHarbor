// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Static check: every t(...) call in the frontend must resolve against
// the en locale. Walks src/frontend for .ts/.tsx, extracts each
// useTranslation binding (namespace(s) + destructured alias), then
// checks every literal-string call against the merged key set for
// that namespace. Static-only: dynamic keys (template literals with
// interpolation, variables) are skipped — they can't be verified
// without running the code.
//
// Failures are formatted as `MISSING <file>:<line>  key=<x>  ns=<y>`
// so the build agent can grep them. Exits 1 on any miss.

import console from 'node:console';
import process from 'node:process';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(scriptDir, '..');
const repoRoot = resolve(frontendRoot, '..', '..');
const localesRoot = join(frontendRoot, 'core', 'i18n', 'locales');
const widgetsRoot = join(repoRoot, 'widgets');
const nodesRoot = join(repoRoot, 'nodes');
const referenceLocale = 'en';

const EXTS = new Set(['.ts', '.tsx']);
const SKIP_DIRS = new Set(['node_modules', 'coverage', 'dist', '.git', 'scripts', '__tests__', '__mocks__']);

function flattenKeys(value, prefix = '', out = new Set()) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    if (prefix) out.add(prefix);
    return out;
  }
  for (const [key, nested] of Object.entries(value)) {
    const next = prefix ? `${prefix}.${key}` : key;
    flattenKeys(nested, next, out);
  }
  return out;
}

function deepMerge(target, source) {
  for (const [key, sv] of Object.entries(source)) {
    const tv = target[key];
    if (sv && typeof sv === 'object' && !Array.isArray(sv) && tv && typeof tv === 'object' && !Array.isArray(tv)) {
      deepMerge(tv, sv);
    } else {
      target[key] = sv;
    }
  }
  return target;
}

function readJson(path) {
  const raw = readFileSync(path, 'utf8');
  // Strip UTF-8 BOM if present.
  return JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function collectCoLocated(rootDir) {
  const merged = {};
  if (!existsDir(rootDir)) return merged;
  for (const slug of readdirSync(rootDir)) {
    const file = join(rootDir, slug, 'i18n', `${referenceLocale}.json`);
    if (!existsFile(file)) continue;
    deepMerge(merged, readJson(file));
  }
  return merged;
}

function existsDir(p) {
  try {
    return statSync(p).isDirectory();
  } catch {
    return false;
  }
}
function existsFile(p) {
  try {
    return statSync(p).isFile();
  } catch {
    return false;
  }
}

function buildNamespaceKeyMap() {
  const map = new Map();
  const enDir = join(localesRoot, referenceLocale);
  for (const file of readdirSync(enDir).filter((f) => f.endsWith('.json'))) {
    const ns = file.replace(/\.json$/, '');
    map.set(ns, flattenKeys(readJson(join(enDir, file))));
  }
  // dashboard absorbs widget translations at runtime (see
  // src/frontend/modules/dashboard/widgets/i18n-loader.ts); mirror
  // that merge here so widget-defined keys don't false-positive.
  // MERGE widget keys into the locale file's parsed object — don't
  // replace it — otherwise host keys like `dashboard.title` are lost.
  if (map.has('dashboard')) {
    const widgetTranslations = collectCoLocated(widgetsRoot);
    if (widgetTranslations) {
      const merged = readJson(join(enDir, 'dashboard.json'));
      deepMerge(merged, widgetTranslations);
      map.set('dashboard', flattenKeys(merged));
    }
  }
  // nodes is a runtime-only namespace produced by /nodes/i18n-loader.ts;
  // the host has now grown a static nodes.json base (mirroring the
  // dashboard pattern) so t(...) lookups via `t('nodes:foo')` resolve
  // against the union of both. Seed the map with whichever exists.
  const nodeBase = existsFile(join(enDir, 'nodes.json')) ? readJson(join(enDir, 'nodes.json')) : {};
  const nodeTranslations = collectCoLocated(nodesRoot);
  if (nodeTranslations) deepMerge(nodeBase, nodeTranslations);
  map.set('nodes', flattenKeys(nodeBase));
  return map;
}

function walk(dir, out = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (SKIP_DIRS.has(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (EXTS.has(full.slice(full.lastIndexOf('.')))) out.push(full);
  }
  return out;
}

// Match a useTranslation(<args>) call's argument list and pull out
// string literals. Supports:
//   'foo' / "foo" / `foo`            → single namespace
//   ['foo','bar'] / ["foo","bar"]    → array of namespaces
// Returns null when args aren't all literal strings (e.g. variable).
function parseNsArgs(args) {
  const trimmed = args.trim();
  if (!trimmed) return ['common'];
  // Array form: ['a', 'b', `c`]
  const arrayMatch = /^\[(.*)\]$/s.exec(trimmed);
  if (arrayMatch) {
    const inner = arrayMatch[1];
    const nsList = [];
    const re = /['"`]([^'"`$\\]+)['"`]/g;
    let m;
    let last = 0;
    while ((m = re.exec(inner)) !== null) {
      nsList.push(m[1]);
      last = re.lastIndex;
    }
    // Any leftover non-whitespace, non-comma content means dynamic.
    const residue = inner.slice(last).replace(/[\s,]/g, '');
    if (residue.length > 0) return null;
    if (nsList.length === 0) return null;
    return nsList;
  }
  // Single string literal.
  const single = /^['"`]([^'"`$\\]+)['"`]$/.exec(trimmed);
  if (single) return [single[1]];
  return null;
}

function parseKeyArg(arg) {
  const trimmed = arg.trim();
  // String literal: 'a.b.c' or "a.b.c"
  let m = /^['"]([^'"$\\]+)['"]$/.exec(trimmed);
  if (m) return { ns: null, key: m[1] };
  // Template literal: `a.b.c` (no interpolation)
  m = /^`([^`$\\]+)`$/.exec(trimmed);
  if (m) return { ns: null, key: m[1] };
  return null;
}

function splitColonKey(key) {
  const colonAt = key.indexOf(':');
  if (colonAt === -1) return { ns: null, key };
  return { ns: key.slice(0, colonAt), key: key.slice(colonAt + 1) };
}

function checkFile(filePath, nsMap, misses) {
  // Scan the whole file as text so calls whose argument list (or
  // options object) wraps onto subsequent lines are still caught —
  // the previous line-based regex missed any `t(<key>,\n  {...})`
  // form, leaving keys with such wrapping to silently pass through
  // the gate. We balance parens across newlines ourselves.
  let text = readFileSync(filePath, 'utf8');
  // Strip /* ... */ block comments (including JSDoc /** ... */) so
  // docstring examples like `t('severity.<level>')` aren't picked up
  // as real calls. Per-line `//` is folded in too. The codebase doesn't
  // embed t(...) inside string literals, so a plain regex strip is
  // safe enough; if it ever isn't, the false positive surfaces as a
  // key path containing characters no real namespace has (`<`, `>`).
  text = text.replace(/\/\*[\s\S]*?\*\//g, (block) => block.replace(/[^\n]/g, ' '));
  text = text.replace(/(^|[^:])\/\/[^\n]*/gm, (m) => (m[0] === '/' ? '' : m[0]));
  const aliases = new Map(); // alias → [ns, ...]

  // First pass: collect every useTranslation binding in the file so
  // later t(...) calls know which namespace(s) they resolve against.
  const bindRe = /\b(?:const|let|var)\s*\{\s*t\s*(?::\s*([A-Za-z_$][\w$]*))?\s*\}\s*=\s*useTranslation\s*\(([^)]*)\)/g;
  let bindMatch;
  while ((bindMatch = bindRe.exec(text)) !== null) {
    const alias = bindMatch[1] || 't';
    const namespaces = parseNsArgs(bindMatch[2]);
    if (!namespaces) continue;
    aliases.set(alias, namespaces);
  }

  function offsetToLine(off) {
    let line = 1;
    for (let i = 0; i < off && i < text.length; i += 1) {
      if (text[i] === '\n') line += 1;
    }
    return line;
  }

  // Walk every `<alias>(` opening, then balance parens across newlines
  // to find the matching close.
  function findMatchingClose(openIdx) {
    let depth = 1;
    let i = openIdx;
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
      if (c === '(') {
        depth += 1;
        i += 1;
        continue;
      }
      if (c === ')') {
        depth -= 1;
        i += 1;
        if (depth === 0) return i - 1;
        continue;
      }
      i += 1;
    }
    return -1;
  }

  function scanCalls(alias, namespaces) {
    const openRe = new RegExp(`\\b${alias}\\s*\\(`, 'g');
    let m;
    while ((m = openRe.exec(text)) !== null) {
      // openRe matches `alias(` so the `(` itself sits at the end.
      const argsStart = m.index + m[0].length;
      const closeIdx = findMatchingClose(argsStart);
      if (closeIdx === -1) break;
      const fullArgs = text.slice(argsStart, closeIdx);
      const commaAt = findTopLevelComma(fullArgs);
      const keyArg = commaAt === -1 ? fullArgs : fullArgs.slice(0, commaAt);
      const optionsText = commaAt === -1 ? '' : fullArgs.slice(commaAt + 1);
      const parsed = parseKeyArg(keyArg);
      if (!parsed) {
        openRe.lastIndex = closeIdx + 1;
        continue;
      }
      if (/returnObjects\s*:\s*true/.test(optionsText)) {
        openRe.lastIndex = closeIdx + 1;
        continue;
      }
      const { ns: explicitNs, key } = splitColonKey(parsed.key);
      const candidateNamespaces = explicitNs ? [explicitNs] : namespaces;
      const hits = candidateNamespaces.some((ns) => nsMap.get(ns)?.has(key));
      if (!hits) {
        misses.push({
          file: relative(frontendRoot, filePath),
          line: offsetToLine(m.index),
          key,
          ns: candidateNamespaces.join('|'),
        });
      }
      openRe.lastIndex = closeIdx + 1;
    }
  }

  function scanI18nT() {
    // Bare `i18n.t(<args>)` calls — only meaningful with the explicit
    // colon form (`i18n.t('ns:key')`). Without a namespace binding
    // there's nothing to resolve a bare key against.
    const openRe = /\bi18n\s*\.\s*t\s*\(/g;
    let m;
    while ((m = openRe.exec(text)) !== null) {
      const argsStart = m.index + m[0].length;
      const closeIdx = findMatchingClose(argsStart);
      if (closeIdx === -1) break;
      const fullArgs = text.slice(argsStart, closeIdx);
      const commaAt = findTopLevelComma(fullArgs);
      const keyArg = commaAt === -1 ? fullArgs : fullArgs.slice(0, commaAt);
      const optionsText = commaAt === -1 ? '' : fullArgs.slice(commaAt + 1);
      if (/returnObjects\s*:\s*true/.test(optionsText)) {
        openRe.lastIndex = closeIdx + 1;
        continue;
      }
      const parsed = parseKeyArg(keyArg);
      if (!parsed) {
        openRe.lastIndex = closeIdx + 1;
        continue;
      }
      const { ns: explicitNs, key } = splitColonKey(parsed.key);
      if (!explicitNs) {
        openRe.lastIndex = closeIdx + 1;
        continue;
      }
      const hits = nsMap.get(explicitNs)?.has(key);
      if (!hits) {
        misses.push({
          file: relative(frontendRoot, filePath),
          line: offsetToLine(m.index),
          key,
          ns: explicitNs,
        });
      }
      openRe.lastIndex = closeIdx + 1;
    }
  }

  for (const [alias, namespaces] of aliases) {
    scanCalls(alias, namespaces);
  }
  scanI18nT();
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
  const nsMap = buildNamespaceKeyMap();
  const misses = [];
  for (const file of walk(frontendRoot)) {
    checkFile(file, nsMap, misses);
  }
  if (misses.length === 0) {
    console.log(`i18n key resolution passed (${nsMap.size} namespaces)`);
    return;
  }
  console.error(`i18n key resolution failed: ${misses.length} miss(es)`);
  for (const m of misses) {
    console.error(`MISSING ${m.file}:${m.line}  key='${m.key}'  ns='${m.ns}'`);
  }
  process.exit(1);
}

main();
