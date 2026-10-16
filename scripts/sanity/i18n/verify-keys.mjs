// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Resolve every literal t('…') key in a TS/TSX file against the en bundle.
 *
 * Catches the three ways i18n keys drift out of the bundles:
 *   1. a key that never existed  — the UI renders the raw dotted key
 *   2. a key that lands on an object rather than a leaf string
 *   3. a defaultValue that hides either of the above
 *
 * Usage:
 *   node scripts/sanity/i18n/verify-keys.mjs <file> [<file> …]
 *   node scripts/sanity/i18n/verify-keys.mjs            # sweep the frontend
 *
 * Exit code 1 when something is unresolved, 0 otherwise. Fired per-edit by
 * scripts/hooks/lint-edited.mjs; run by hand before a commit that touches i18n.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

// MCHARBOR_VERIFY_KEYS_ROOT points the resolver at a fixture tree; the hook tests use it.
const REPO = process.env.MCHARBOR_VERIFY_KEYS_ROOT
  ? resolve(process.env.MCHARBOR_VERIFY_KEYS_ROOT)
  : resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..');
const FRONTEND = join(REPO, 'src', 'frontend');
const EN = join(FRONTEND, 'core', 'i18n', 'locales', 'en');
const WIDGETS_DIR = join(REPO, 'widgets');
const NODES_DIR = join(REPO, 'nodes');
const PLUGINS_ROOT = join(REPO, 'src', 'plugins', 'Plugins');

/* ─── the en bundle, flattened to dotted paths ──────────────────── */

function flatten(node, prefix, out) {
  for (const [k, v] of Object.entries(node)) {
    const path = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      if (out.get(path) == null) out.set(path, null); // object path; never clobber a string another plugin put here
      flatten(v, path, out);
    } else {
      out.set(path, String(v));
    }
  }
  return out;
}

const bundles = new Map();
for (const file of readdirSync(EN)) {
  if (!file.endsWith('.json')) continue;
  const ns = file.slice(0, -5);
  bundles.set(ns, flatten(JSON.parse(readFileSync(join(EN, file), 'utf8')), '', new Map()));
}

// Widgets and nodes ship their own en bundle and are merged into an existing
// namespace at runtime (see core/i18n/i18n.ts): a widget's keys land in
// `dashboard`, a node's in `nodes`. Without this the resolver reports every
// widget and node string as a missing key.
function mergeCatalog(dir, ns, prefixWithFolder = false) {
  let entries;
  try {
    entries = readdirSync(join(REPO, dir), { withFileTypes: true });
  } catch {
    return;
  }
  const target = bundles.get(ns) ?? new Map();
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const file = join(REPO, dir, entry.name, 'i18n', 'en.json');
    try {
      flatten(JSON.parse(readFileSync(file, 'utf8')), prefixWithFolder ? entry.name : '', target);
    } catch {
      // a catalog folder without an en bundle is the catalog's problem
    }
  }
  bundles.set(ns, target);
}
mergeCatalog('widgets', 'dashboard');
mergeCatalog('nodes', 'nodes', true);

// A plugin ships its own namespace bundles under <plugin>/i18n/en/<ns>.json —
// one file per namespace, unlike the single-file widget/node catalogs above —
// so a plugin frontend file's t() calls resolve in --hook mode instead of
// reporting every plugin string as missing.
//
// At runtime the host serves a plugin differently (plugin_runtime/service.go): every
// file under i18n/<lang>/ is flattened into ONE bundle, registered under the plugin's
// slug and under each name its plugin.yaml lists in i18n_namespaces — so
// useTranslation('storage-ftp') reads i18n/en/common.json. Both views are registered,
// the per-file one for the files that already rely on it.
const HOST_NAMESPACES = new Set([
  'common', 'auth', 'containers', 'images', 'volumes', 'networks', 'stacks', 'environments',
  'settings', 'dashboard', 'terminal', 'security', 'docker', 'host', 'gitops', 'abac', 'ai',
  'plugins', 'enterprise', 'scim', 'nodes',
]);

function declaredNamespaces(pluginDir) {
  let yaml;
  try {
    yaml = readFileSync(join(pluginDir, 'plugin.yaml'), 'utf8');
  } catch {
    return [];
  }
  const out = [];
  const block = yaml.match(/^([ \t]*)i18n_namespaces:[ \t]*(\[[^\]\n]*\])?[ \t]*\r?\n?((?:\1[ \t]*-[^\n]*\r?\n?)*)/m);
  if (!block) return out;
  const items = block[2] ? block[2].slice(1, -1).split(',') : block[3].split('\n').map((l) => l.replace(/^\s*-\s*/, ''));
  for (const raw of items) {
    const ns = raw.trim().replace(/^['"]|['"]$/g, '');
    if (ns) out.push(ns);
  }
  return out;
}

function mergePluginCatalogs() {
  let plugins;
  try {
    plugins = readdirSync(PLUGINS_ROOT, { withFileTypes: true });
  } catch {
    return;
  }
  for (const plugin of plugins) {
    if (!plugin.isDirectory()) continue;
    const enDir = join(PLUGINS_ROOT, plugin.name, 'i18n', 'en');
    let files;
    try {
      files = readdirSync(enDir);
    } catch {
      continue;
    }
    const merged = new Map();
    for (const file of files) {
      if (!file.endsWith('.json')) continue;
      const ns = file.slice(0, -5);
      const target = bundles.get(ns) ?? new Map();
      try {
        const tree = JSON.parse(readFileSync(join(enDir, file), 'utf8'));
        flatten(tree, '', target);
        flatten(tree, '', merged);
      } catch {
        // a plugin bundle that fails to parse is the plugin's problem
      }
      bundles.set(ns, target);
    }
    const served = [plugin.name, ...declaredNamespaces(join(PLUGINS_ROOT, plugin.name))];
    for (const ns of new Set(served)) {
      if (HOST_NAMESPACES.has(ns)) continue;
      const target = bundles.get(ns) ?? new Map();
      for (const [k, v] of merged) if (target.get(k) == null) target.set(k, v);
      bundles.set(ns, target);
    }
  }
}
mergePluginCatalogs();

/* ─── comments, blanked in place ────────────────────────────────── */

// A JSDoc line like `default is t('severity.<level>')` is documentation, not a
// call site. Blanking rather than deleting keeps every later offset — and so
// every reported line number — exactly where it was.
const COMMENT = /\/\*[\s\S]*?\*\/|(^|[^:"'`\\])\/\/[^\n]*/g;

function blankComments(source) {
  return source.replace(COMMENT, (match, lead = '') => {
    const body = match.slice(lead.length);
    return lead + [...body].map((c) => (c === '\n' ? '\n' : ' ')).join('');
  });
}

/* ─── which namespace does each t-binding carry ─────────────────── */

// const { t } = useTranslation('containers')
// const { t: tc } = useTranslation('common')
// const { t } = useTranslation(['common', 'nodes'])
// `const { t } = …`, `const { t: tc } = …`, and `const { t, i18n } = …` alike:
// the destructuring list is captured whole and scanned for t, because a
// pattern anchored on `{ t }` alone silently skipped every file that also
// pulled `i18n` off the hook - and skipped files are reported as clean.
const BINDING = /\{([^{}]*)\}\s*=\s*useTranslation\(\s*(\[[^\]]*\]|'[^']*'|"[^"]*")/g;
const T_IN_DESTRUCTURE = /(^|,)\s*t\s*(?::\s*(\w+))?\s*(?=,|$)/;

function bindingsOf(source) {
  const found = new Map();
  for (const m of source.matchAll(BINDING)) {
    const bound = T_IN_DESTRUCTURE.exec(m[1]);
    if (!bound) continue; // this hook call does not bind t at all
    const alias = bound[2] ?? 't';
    const namespaces = [...m[2].matchAll(/['"]([^'"]+)['"]/g)].map((n) => n[1]);
    const prior = found.get(alias) ?? [];
    // A file may bind the same alias twice (one hook per exported component).
    // Both namespaces are legitimate for that alias, so keep the union.
    found.set(alias, [...new Set([...prior, ...namespaces])]);
  }
  return found;
}

/* ─── the call text, from '(' to its matching ')' ───────────────── */

function callText(source, openParen) {
  let depth = 0;
  for (let i = openParen; i < source.length && i < openParen + 2000; i += 1) {
    const c = source[i];
    if (c === '(') depth += 1;
    else if (c === ')') {
      depth -= 1;
      if (depth === 0) return source.slice(openParen + 1, i);
    }
  }
  return source.slice(openParen + 1, openParen + 2000);
}

function lineOf(source, index) {
  return source.slice(0, index).split('\n').length;
}

/* ─── the check ─────────────────────────────────────────────────── */

// An en value that is just its own last key segment ("exit" for columns.exit) reads
// exactly like a failed lookup on screen. i18n check 13 reports the same thing over the
// whole bundle and reads the same accept list, so one entry silences both.
let echoAccepted = {};
try {
  echoAccepted = JSON.parse(readFileSync(join(REPO, 'scripts', 'sanity', 'i18n', 'echo-accepted.json'), 'utf8'));
} catch {
  // No accept list: every echo is reported, which is the safe direction.
}

function echoesKey(ns, key, value) {
  if (typeof value !== 'string' || value !== key.split('.').pop()) return false;
  const accepted = echoAccepted[`${ns}:${key}`];
  return !(accepted && accepted.value === value);
}



/**
 * The literal keys a conditional first argument can evaluate to, or null when
 * a branch is not a plain string.
 *
 * `t(hasUpdate ? 'store.updateAction' : 'store.install')` is fully checkable:
 * the condition can be anything, but both branches are literals, so both are
 * resolvable. A single literal is the only shape the resolver used to
 * understand, so every one of these went unchecked.
 */
function literalAlternatives(text) {
  const arg = firstArgument(text);
  if (!arg || !arg.includes('?')) return null;
  const out = [];
  return collectBranches(arg, out) ? [...new Set(out)] : null;
}

/** Walk a (possibly nested) ternary, collecting literal branches. */
function collectBranches(expr, out) {
  const trimmed = expr.trim();
  const literal = trimmed.match(/^(['"])([^'"]*)\1$/);
  if (literal) {
    out.push(literal[2]);
    return true;
  }
  const q = topLevelIndex(trimmed, '?');
  if (q < 0) return false;
  const colon = topLevelIndex(trimmed, ':', q + 1);
  if (colon < 0) return false;
  return (
    collectBranches(trimmed.slice(q + 1, colon), out) &&
    collectBranches(trimmed.slice(colon + 1), out)
  );
}

/** Index of `char` at nesting depth 0, ignoring quoted spans. */
function topLevelIndex(text, char, from = 0) {
  let depth = 0;
  let quote = null;
  for (let i = from; i < text.length; i += 1) {
    const c = text[i];
    if (quote) {
      if (c === '\\') i += 1;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') quote = c;
    else if ('([{'.includes(c)) depth += 1;
    else if (')]}'.includes(c)) depth -= 1;
    else if (c === char && depth === 0) return i;
  }
  return -1;
}

/** The first argument of a call's argument list, split on the top-level comma. */
function firstArgument(text) {
  let depth = 0;
  let quote = null;
  for (let i = 0; i < text.length; i += 1) {
    const c = text[i];
    if (quote) {
      if (c === '\\') i += 1;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') quote = c;
    else if ('([{'.includes(c)) depth += 1;
    else if (')]}'.includes(c)) depth -= 1;
    else if (c === ',' && depth === 0) return text.slice(0, i);
  }
  return text;
}


/**
 * Literal t('...') keys in a file that receives `t` as a prop.
 *
 * The namespace belongs to whichever component passed `t` down, so a key
 * cannot be resolved precisely here. A key that appears in no bundle under any
 * namespace is still unambiguously broken, and checking only that keeps the
 * check free of false positives.
 */
function orphanKeys(source, problems) {
  const LITERAL_CALL = /(?<![\w$.])t\(\s*(['"])([^'"\n]+)\1/g;
  for (const m of source.matchAll(LITERAL_CALL)) {
    let key = m[2];
    const colon = key.indexOf(':');
    let candidates = [...bundles.keys()];
    if (colon > 0) {
      candidates = [key.slice(0, colon)];
      key = key.slice(colon + 1);
    }
    if (!/^[A-Za-z0-9_][A-Za-z0-9_.-]*$/.test(key)) continue;
    const found = candidates.some((ns) => {
      const bundle = bundles.get(ns);
      if (!bundle) return false;
      if (bundle.has(key) && bundle.get(key) !== null) return true;
      return PLURAL_SUFFIXES.some((suffix) => bundle.has(`${key}${suffix}`));
    });
    if (!found) {
      problems.push({
        line: lineOf(source, m.index),
        key: m[2],
        kind: 'orphan',
        detail: colon > 0 ? candidates[0] : 'any namespace',
      });
    }
  }
  return problems;
}

/** Which namespaces a call's key is resolved against. */
function namespacesFor(text, bindings, alias) {
  const nsOption = text.match(/\bns\s*:\s*(['"])([^'"]+)\1/);
  if (nsOption) return [nsOption[2]];
  return bindings.get(alias) ?? [];
}

// A dotted, lowercase-initial prefix ending in a dot: the fixed part of a
// computed key. URLs, file paths and css selectors will not match.
const KEY_PREFIX = /^[a-z][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]+)*\.$/;

function prefixFromTemplate(tpl) {
  const m = tpl.match(/^\s*`([^`]*?)\$\{/);
  if (!m) return null;
  return KEY_PREFIX.test(m[1]) ? m[1] : null;
}

/**
 * The static prefix of a computed key, whether written inline or built by a
 * helper in the same file.
 *
 * `t(labelKey(pos))` is the shape that hid the missing notifications.position.*
 * group: it is neither a literal nor a template at the call site, so every
 * check looked straight past it - it did not even reach the computed-key
 * inventory.
 */
function keyPrefixOf(text, source) {
  const inline = prefixFromTemplate(text);
  if (inline) return inline;

  const call = text.match(/^\s*([A-Za-z_$][\w$]*)\s*\(/);
  if (!call) return null;
  const name = call[1];
  const decl = new RegExp(
    '(?:function\\s+' + name + '\\s*\\(|(?:const|let|var)\\s+' + name + '\\s*=)',
  ).exec(source);
  if (!decl) return null;
  const body = source.slice(decl.index, decl.index + 600);
  const tpl = body.match(/return\s+(`[^`]*`)/) || body.match(/=>\s*(`[^`]*`)/);
  return tpl ? prefixFromTemplate(tpl[1]) : null;
}

// i18next stores a counted key under its CLDR plural forms only; the call site
// names the bare key and passes `count`.
const PLURAL_SUFFIXES = ['_zero', '_one', '_two', '_few', '_many', '_other'];

// openSetMarked reports whether an `i18n-open-set:` exemption sits on the call's
// own lines or on the line above any of them (the eslint-disable-next-line form).
function openSetMarked(rawLines, startLine, text) {
  const last = startLine + text.split('\n').length - 1;
  for (let n = startLine - 1; n <= last; n += 1) {
    if (/i18n-open-set:\s*\S/.test(rawLines[n - 1] ?? '')) return true;
  }
  return false;
}

function check(file) {
  const raw = readFileSync(file, 'utf8');
  const rawLines = raw.split('\n');
  const source = blankComments(raw);
  const bindings = bindingsOf(source);
  const problems = [];

  // A file with no useTranslation receives t as a prop; its namespace is the
  // caller's and is not knowable from here. The key still is, though: if a
  // literal key exists in NO namespace at all, no caller's namespace can
  // rescue it. That is conservative - it cannot produce a false positive - and
  // it covers ~400 calls that were skipped outright.
  if (bindings.size === 0) return orphanKeys(source, problems);

  const aliases = [...bindings.keys()];
  const CALL = new RegExp(`\\b(${aliases.join('|')})\\(`, 'g');

  for (const m of source.matchAll(CALL)) {
    const alias = m[1];
    const open = m.index + m[0].length - 1;
    const text = callText(source, open);

    const literal = text.match(/^\s*(['"])([^'"]+)\1/);
    if (!literal) {
      // A computed key. The values are unknowable, but the static prefix is
      // not: `notifications.position.${p}` can only ever resolve under
      // `notifications.position.`, so an empty group there means every value
      // renders the raw key. Rule 6 requires the whole group to exist; this is
      // what enforces it.
      // t(cond ? 'a.b' : 'c.d') - every branch is a literal, so every branch is
      // resolvable. Only when the whole first argument is built from quoted
      // strings and a conditional; anything with an identifier in it is not.
      const alternatives = literalAlternatives(text);
      if (alternatives) {
        const namespaces = namespacesFor(text, bindings, alias);
        for (const candidate of alternatives) {
          let ns = namespaces;
          let key = candidate;
          const colon = candidate.indexOf(':');
          if (colon > 0) { ns = [candidate.slice(0, colon)]; key = candidate.slice(colon + 1); }
          const ok = ns.some((n) => {
            const bundle = bundles.get(n);
            if (!bundle) return false;
            if (bundle.has(key) && bundle.get(key) !== null) return true;
            return PLURAL_SUFFIXES.some((suffix) => bundle.has(`${key}${suffix}`));
          });
          if (!ok) {
            problems.push({
              line: lineOf(source, m.index),
              key: candidate,
              kind: 'missing',
              detail: ns.join('|'),
            });
          }
        }
        continue;
      }

      const prefix = keyPrefixOf(text, source);

      // A defaultValue on a computed key is the one place rule 4 has a real
      // tension: when the value set is open-ended (a plugin label, a backend
      // step name), the default is the only sane fallback and rule 6 asks for
      // an explicit literal mapping instead - a refactor, not a one-line fix.
      // So it is reported for a human rather than failed: hard-failing ~55
      // mostly-legitimate sites is how a check gets dismissed wholesale. When
      // the group is missing entirely the prefix finding below fires instead,
      // and that one is unambiguous.
      if (/\bdefaultValue\s*:/.test(text) && !openSetMarked(rawLines, lineOf(source, m.index), text)) {
        problems.push({
          line: lineOf(source, m.index),
          key: prefix ? prefix + '*' : '(computed)',
          kind: 'computedDefault',
        });
      }

      if (prefix) {
        const namespaces = namespacesFor(text, bindings, alias);
        const known = namespaces.some((ns) => {
          const bundle = bundles.get(ns);
          if (!bundle) return false;
          for (const k of bundle.keys()) if (k.startsWith(prefix)) return true;
          return false;
        });
        if (!known) {
          problems.push({
            line: lineOf(source, m.index),
            key: prefix + '*',
            kind: 'prefix',
            detail: namespaces.join('|'),
          });
        }
      }
      continue;
    }

    const line = lineOf(source, m.index);
    const raw = literal[2];

    // The second positional argument and the defaultValue option both render a
    // string whether or not the key resolves, which is exactly how a missing
    // key survives review.
    const afterKey = text.slice(literal[0].length);
    if (/^\s*,\s*['"]/.test(afterKey) || /\bdefaultValue\s*:/.test(text)) {
      problems.push({ line, key: raw, kind: 'defaultValue' });
    }

    // t('ns:key') beats the hook's namespace; { ns: 'x' } beats it too.
    let namespaces;
    let key = raw;
    const prefixed = raw.indexOf(':');
    if (prefixed > 0) {
      namespaces = [raw.slice(0, prefixed)];
      key = raw.slice(prefixed + 1);
    } else {
      const nsOption = text.match(/\bns\s*:\s*(['"])([^'"]+)\1/);
      namespaces = nsOption ? [nsOption[2]] : (bindings.get(alias) ?? []);
    }

    const unknownNs = namespaces.filter((ns) => !bundles.has(ns));
    if (unknownNs.length === namespaces.length) {
      // Every candidate namespace is unknown — a plugin bundle registered at
      // runtime, or a typo. Reported, not resolved.
      problems.push({ line, key: raw, kind: 'namespace', detail: unknownNs.join(', ') });
      continue;
    }

    let resolved = false;
    let object = false;
    const echoed = [];
    for (const ns of namespaces) {
      const bundle = bundles.get(ns);
      if (!bundle) continue;
      if (bundle.has(key) && echoesKey(ns, key, bundle.get(key))) echoed.push(ns);
      if (!bundle.has(key)) {
        // A counted key lives in the bundle only under its plural suffixes:
        // i18next picks the form at runtime from `count`, so `sessions.count`
        // resolving via `sessions.count_one` is correct, not a miss.
        if (PLURAL_SUFFIXES.some((suffix) => bundle.has(`${key}${suffix}`))) resolved = true;
        continue;
      }
      if (bundle.get(key) === null) object = true;
      else resolved = true;
    }

    // returnObjects is how you ask for the object on purpose.
    if (object && /\breturnObjects\s*:\s*true/.test(text)) continue;
    if (object && !resolved) {
      problems.push({ line, key: raw, kind: 'object', detail: namespaces.join('|') });
      continue;
    }
    if (!resolved) {
      problems.push({ line, key: raw, kind: 'missing', detail: namespaces.join('|') });
    } else if (echoed.length) {
      problems.push({ line, key: raw, kind: 'echo', detail: echoed[0] });
    }
  }

  return problems;
}

/* ─── targets ───────────────────────────────────────────────────── */

function walk(dir, out) {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'dist' || entry === '.vite') continue;
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) walk(path, out);
    else if (/\.tsx?$/.test(entry)) out.push(path);
  }
  return out;
}

// --hook: read the PostToolUse payload on stdin and check the file it names.
// Parsing the JSON here rather than in the shell keeps the hook free of a jq
// dependency, which is not installed on every dev machine.
function hookTarget() {
  let payload;
  try {
    payload = JSON.parse(readFileSync(0, 'utf8'));
  } catch {
    return [];
  }
  const file = payload?.tool_input?.file_path;
  if (typeof file !== 'string' || !/\.tsx?$/.test(file)) return [];
  const abs = resolve(file);
  // The host frontend, the widget/node catalogs, and a plugin's own frontend
  // source — the three places outside the app itself a t() call site lives.
  const roots = [FRONTEND, WIDGETS_DIR, NODES_DIR];
  if (roots.some((root) => abs.startsWith(root + sep))) return [abs];
  const inPluginFrontend = /[\\/]src[\\/]plugins[\\/]Plugins[\\/][^\\/]+[\\/]frontend[\\/]src[\\/]/.test(abs);
  return inPluginFrontend ? [abs] : [];
}

const args = process.argv.slice(2);
const hookMode = args[0] === '--hook';
const jsonMode = args.includes('--json');
const paths = args.filter((a) => !a.startsWith('--'));

// The full sweep covers the catalogs as well as the app: a widget's or node's
// strings resolve against the same merged bundles.
function sweepAll() {
  const out = walk(FRONTEND, []);
  for (const dir of ['widgets', 'nodes']) {
    try {
      walk(join(REPO, dir), out);
    } catch {
      // catalog folder absent in this checkout
    }
  }
  return out.filter((f) => !/\.(test|stories)\.tsx?$|\.d\.ts$/.test(f));
}

const targets = hookMode
  ? hookTarget()
  : paths.length
    ? paths.map((a) => resolve(a)).filter((a) => /\.tsx?$/.test(a))
    : sweepAll();

const MESSAGES = {
  missing: (p) => `key not in the en bundle (${p.detail}) — add it to locales/*/${p.detail.split('|')[0]}.json, or fix the key`,
  object: (p) => `resolves to an object, not a string (${p.detail}) — point at a leaf key`,
  namespace: (p) => `namespace "${p.detail}" has no en bundle`,
  defaultValue: () => 'defaultValue is banned — the key must exist in the bundle',
  computedDefault: () =>
    'defaultValue on a computed key — confirm the value set is genuinely open-ended; if it is closed, map it to literal keys (rule 6)',
  orphan: (p) =>
    `key exists in no namespace (${p.detail}) — this file receives t as a prop, so the namespace is the caller's, but no bundle anywhere defines this key`,
  echo: (p) =>
    `the en value (${p.detail}) is the key's own last segment, which reads like a failed lookup — write the real string, or list "${p.detail}:${p.key.replace(/^[^:]+:/, '')}" in scripts/sanity/i18n/echo-accepted.json`,
  prefix: (p) => `computed key: nothing in the en bundle (${p.detail}) starts with this prefix — the whole group is missing, so every value renders the raw key`,
};

// Kinds that are an unambiguous defect. `computedDefault` is deliberately not
// here: it is a judgement call reported for review (see the comment at the
// call site), and a hook that shouts on 55 mostly-legitimate sites is a hook
// people learn to ignore.
const FAILING = new Set(['missing', 'object', 'namespace', 'defaultValue', 'prefix', 'orphan']);

let failed = 0;
const lines = [];
const warnings = [];
const records = [];
for (const file of targets) {
  let problems;
  try {
    problems = check(file);
  } catch {
    continue; // unreadable or deleted between glob and read
  }
  if (!problems.length) continue;
  const shown = relative(REPO, file).split(sep).join('/');
  for (const p of problems) {
    const message = MESSAGES[p.kind](p);
    records.push({ file: shown, line: p.line, key: p.key, kind: p.kind, detail: p.detail ?? '', message });
    if (!FAILING.has(p.kind)) {
      if (p.kind === 'echo') warnings.push(`${shown}:${p.line}  ${p.key}  — ${message}`);
      continue;
    }
    failed += 1;
    lines.push(`${shown}:${p.line}  ${p.key}  — ${message}`);
  }
}

// --json is how scripts/sanity/i18n/check.ps1 consumes this file. The sweep
// used to carry its own PowerShell resolver; it silently skipped every call
// with a defaultValue and ignored the { ns } option, so it reported 9 problems
// where there were 328. One resolver, one answer.
if (jsonMode) {
  process.stdout.write(JSON.stringify(records));
  process.exit(0);
}

// In hook mode this lands in the agent's transcript, so keep it short and
// never fail the tool call — the edit already happened.
if (hookMode) {
  if (failed) {
    console.error('⚠️  i18n keys — see .claude/rules/i18n-keys.md');
    for (const line of lines.slice(0, 12)) console.error(`   ${line}`);
    if (lines.length > 12) console.error(`   …and ${lines.length - 12} more`);
  }
  if (warnings.length) {
    console.error('i18n key warnings (not failing):');
    for (const line of warnings.slice(0, 8)) console.error(`   ${line}`);
  }
  process.exit(0);
}

for (const line of warnings) console.error(`warning: ${line}`);
if (!failed) process.exit(0);

for (const line of lines) console.error(line);
console.error(`\n${failed} i18n key problem(s). See .claude/rules/i18n-keys.md.`);
process.exit(1);
