// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Retire every banned `defaultValue` from a t() call site.
 *
 * `.claude/rules/i18n-keys.md` bans defaultValue outright: it renders a string
 * whether or not the key exists, so a missing key looks fine in English and
 * ships as English in the other five locales forever. This script performs the
 * migration the rule implies:
 *
 *   1. read the default's English text out of the call site
 *   2. write it into locales/en/<ns>.json when the key is not there yet
 *   3. delete the defaultValue from the call site
 *
 * A default that is not a plain string literal (a ternary, a template with an
 * expression, an identifier) cannot be harvested and is reported for a human.
 * Plugin bundles registered at runtime are exempt: their namespace is not on
 * disk, so a defaultValue is the only fallback available.
 *
 * Usage:
 *   node scripts/sanity/i18n/adopt-defaults.mjs           # report
 *   node scripts/sanity/i18n/adopt-defaults.mjs --write   # migrate
 */

import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO = resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..');
const FRONTEND = join(REPO, 'src', 'frontend');
const EN = join(FRONTEND, 'core', 'i18n', 'locales', 'en');
const WRITE = process.argv.includes('--write');

// Namespaces a plugin registers at runtime via addResourceBundle have no file
// on disk; a defaultValue there is the documented fallback, not a violation.
const RUNTIME_EXEMPT = new Set(['plugin', 'plugins-runtime']);

/* --- bundles ---------------------------------------------------- */

const bundleFiles = readdirSync(EN).filter((f) => f.endsWith('.json'));
const bundles = new Map(); // ns -> parsed object
for (const file of bundleFiles) {
  bundles.set(file.slice(0, -5), JSON.parse(readFileSync(join(EN, file), 'utf8')));
}

const hasKey = (ns, key) => {
  const root = bundles.get(ns);
  if (!root) return false;
  const node = key.split('.').reduce((a, k) => (a && typeof a === 'object' ? a[k] : undefined), root);
  return typeof node === 'string';
};

const setKey = (ns, key, value) => {
  const root = bundles.get(ns);
  const parts = key.split('.');
  let node = root;
  for (const part of parts.slice(0, -1)) {
    if (!node[part] || typeof node[part] !== 'object') node[part] = {};
    node = node[part];
  }
  node[parts[parts.length - 1]] = value;
};

/* --- parsing (mirrors verify-keys.mjs) -------------------------- */

const COMMENT = /\/\*[\s\S]*?\*\/|(^|[^:"'`\\])\/\/[^\n]*/g;
const blankComments = (s) =>
  s.replace(COMMENT, (match, lead = '') =>
    lead + [...match.slice(lead.length)].map((c) => (c === '\n' ? '\n' : ' ')).join(''));

const BINDING = /\{([^{}]*)\}\s*=\s*useTranslation\(\s*(\[[^\]]*\]|'[^']*'|"[^"]*")/g;
const T_IN_DESTRUCTURE = /(^|,)\s*t\s*(?::\s*(\w+))?\s*(?=,|$)/;
function bindingsOf(source) {
  const found = new Map();
  for (const m of source.matchAll(BINDING)) {
    const bound = T_IN_DESTRUCTURE.exec(m[1]);
    if (!bound) continue;
    const alias = bound[2] ?? 't';
    const namespaces = [...m[2].matchAll(/['"]([^'"]+)['"]/g)].map((n) => n[1]);
    found.set(alias, [...new Set([...(found.get(alias) ?? []), ...namespaces])]);
  }
  return found;
}

/** End index of the argument list opened at `openParen`. */
function callEnd(source, openParen) {
  let depth = 0;
  for (let i = openParen; i < source.length; i += 1) {
    const c = source[i];
    if (c === '(') depth += 1;
    else if (c === ')') {
      depth -= 1;
      if (depth === 0) return i;
    }
  }
  return -1;
}

/** Split an object-literal body on top-level commas, keeping raw slices. */
function splitProps(body) {
  const parts = [];
  let depth = 0;
  let start = 0;
  let quote = null;
  for (let i = 0; i < body.length; i += 1) {
    const c = body[i];
    if (quote) {
      if (c === '\\') i += 1;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') quote = c;
    else if ('([{'.includes(c)) depth += 1;
    else if (')]}'.includes(c)) depth -= 1;
    else if (c === ',' && depth === 0) {
      parts.push(body.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(body.slice(start));
  return parts;
}

/**
 * A template-literal default -> i18next interpolation, when every `${expr}` in
 * it is already being passed as an option. `t('k', { username, defaultValue:
 * `Disable ${username}'s account` })` becomes the bundle string
 * "Disable {{username}}'s account" — the value is already in scope at runtime,
 * so nothing else has to change at the call site.
 *
 * Returns null when an expression is not passed as an option; guessing a name
 * would silently produce a string that interpolates to nothing.
 */
function templateText(expr, keptProps) {
  const trimmed = expr.trim();
  if (!trimmed.startsWith('`') || !trimmed.endsWith('`')) return null;
  const body = trimmed.slice(1, -1);
  if (body.includes('`')) return null;

  // option name -> the source text of the value it is given
  const byValue = new Map();
  for (const prop of keptProps) {
    const text = prop.trim();
    const named = text.match(/^([A-Za-z_$][\w$]*)\s*:\s*([\s\S]+)$/);
    if (named) byValue.set(named[2].trim(), named[1]);
    else if (/^[A-Za-z_$][\w$]*$/.test(text)) byValue.set(text, text); // shorthand
  }

  let failed = false;
  const out = body.replace(/\$\{([^{}]+)\}/g, (_, inner) => {
    const option = byValue.get(inner.trim());
    if (!option) {
      failed = true;
      return '';
    }
    return `{{${option}}}`;
  });
  if (failed) return null;
  return out.replace(/\\`/g, '`').replace(/\\\$/g, '$');
}

/** A plain string literal (possibly 'a' + 'b' across lines) -> its text. */
function literalText(expr) {
  const pieces = [];
  const RE = /^\s*(['"])((?:[^\\]|\\.)*?)\1\s*(\+)?/;
  let rest = expr.trim();
  while (rest.length) {
    const m = rest.match(RE);
    if (!m) return null;
    pieces.push(m[2]);
    rest = rest.slice(m[0].length);
    if (!m[3]) {
      if (rest.trim().length) return null;
      break;
    }
  }
  if (!pieces.length) return null;
  return pieces
    .join('')
    .replace(/\\'/g, "'")
    .replace(/\\"/g, '"')
    .replace(/\\n/g, '\n')
    .replace(/\\\\/g, '\\');
}

/* --- walk ------------------------------------------------------- */

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'dist' || entry === 'locales') continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (/\.(ts|tsx)$/.test(entry)) out.push(full);
  }
  return out;
}

const stats = { migrated: 0, added: 0, dynamic: 0, files: 0 };
const dynamic = [];
const touchedNs = new Set();

for (const file of walk(FRONTEND)) {
  const original = readFileSync(file, 'utf8');
  if (!original.includes('defaultValue')) continue;
  const blanked = blankComments(original);
  const bindings = bindingsOf(blanked);
  if (bindings.size === 0) continue;

  const CALL = new RegExp(`\\b(${[...bindings.keys()].join('|')})\\(`, 'g');
  const edits = [];

  for (const m of blanked.matchAll(CALL)) {
    const open = m.index + m[0].length - 1;
    const close = callEnd(blanked, open);
    if (close < 0) continue;
    const args = blanked.slice(open + 1, close);
    if (!args.includes('defaultValue')) continue;

    const literal = args.match(/^\s*(['"])([^'"]+)\1/);
    if (!literal) continue;
    const raw = literal[2];

    let ns;
    let key = raw;
    const colon = raw.indexOf(':');
    if (colon > 0) {
      ns = raw.slice(0, colon);
      key = raw.slice(colon + 1);
    } else {
      const nsOpt = args.match(/\bns\s*:\s*(['"])([^'"]+)\1/);
      ns = nsOpt ? nsOpt[2] : (bindings.get(m[1]) ?? [])[0];
    }
    if (!ns || RUNTIME_EXEMPT.has(ns) || !bundles.has(ns)) continue;

    const after = args.slice(literal[0].length);
    const braceStart = after.indexOf('{');
    if (braceStart < 0) continue;
    const braceEnd = after.lastIndexOf('}');
    if (braceEnd < braceStart) continue;

    const body = after.slice(braceStart + 1, braceEnd);
    const props = splitProps(body);
    const keptProps = [];
    let defaultExpr = null;
    for (const prop of props) {
      if (/^\s*defaultValue\s*:/.test(prop)) defaultExpr = prop.replace(/^\s*defaultValue\s*:/, '');
      else if (prop.trim().length) keptProps.push(prop);
    }
    if (defaultExpr === null) continue;

    const line = blanked.slice(0, m.index).split('\n').length;
    const text = literalText(defaultExpr) ?? templateText(defaultExpr, keptProps);
    if (text === null) {
      dynamic.push(`${relative(REPO, file)}:${line}  ${ns}:${key}  ${defaultExpr.trim().slice(0, 60).replace(/\s+/g, ' ')}`);
      stats.dynamic += 1;
      continue;
    }

    if (!hasKey(ns, key)) {
      setKey(ns, key, text);
      touchedNs.add(ns);
      stats.added += 1;
    }

    const optionsStart = open + 1 + literal[0].length + braceStart;
    const optionsEnd = open + 1 + literal[0].length + braceEnd + 1;
    const multiline = /\n/.test(body);
    const replacement = keptProps.length
      ? `{${keptProps.join(',').replace(/[\s,]+$/, '')}${multiline ? '\n  ' : ' '}}`
      : '';
    if (replacement) {
      edits.push([optionsStart, optionsEnd, replacement]);
    } else {
      let commaStart = optionsStart - 1;
      while (commaStart > open && /\s/.test(original[commaStart])) commaStart -= 1;
      if (original[commaStart] !== ',') commaStart = optionsStart;
      edits.push([commaStart, optionsEnd, '']);
    }
    stats.migrated += 1;
  }

  if (!edits.length) continue;
  stats.files += 1;
  if (!WRITE) continue;
  let out = original;
  for (const [start, end, text] of edits.sort((a, b) => b[0] - a[0])) {
    out = out.slice(0, start) + text + out.slice(end);
  }
  writeFileSync(file, out.replace(/\r\n/g, '\n'), 'utf8');
}

if (WRITE) {
  for (const ns of touchedNs) {
    const p = join(EN, `${ns}.json`);
    writeFileSync(p, `${JSON.stringify(bundles.get(ns), null, 2)}\n`.replace(/\r\n/g, '\n'), 'utf8');
  }
}

console.log(`defaultValue call sites migrated:  ${stats.migrated} across ${stats.files} file(s)`);
console.log(`keys added to the en bundle:       ${stats.added} (${[...touchedNs].join(', ') || 'none'})`);
console.log(`dynamic defaults left for a human: ${stats.dynamic}`);
for (const d of dynamic) console.log(`  ${d}`);
if (!WRITE) console.log('\nReport only. Re-run with --write to apply, then sync-locales.mjs --write.');
