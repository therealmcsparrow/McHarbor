// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { readdirSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { inChanged } from '../lib/payload.mjs';

const BARE_NAMESPACE = /useTranslation\(\s*\)/;
// Case-sensitive: <Button> and <PlainButton> never match a lowercase tag.
const RAW_BUTTON = /<button(\s|>|\/)/;
const INLINE_STYLE = /style=\{\{/;
const NATIVE_DIALOG = /window\.(?:confirm|alert)\(/;
const DOUBLE_CAST = /as\s+unknown\s+as\s+/;
const RAW_DATE_FORMAT = /\.toLocaleDateString\(\s*\)/;
const QUERY_DROPS_ENVELOPE = /queryFn:.*\.then\(\s*\(?\s*\w*\s*\)?\s*=>\s*\w+\.data\s*\)/;
const GENERIC_ONERROR = /onError:\s*\(\)\s*=>\s*toast\.error\(/;
const HARDCODED_COLOR = /\b(?:bg-white|bg-gray-\d+|text-gray-\d+|bg-black)\b/;
// Family prefixes from .claude/rules/plugin-authoring.md — a literal slug
// naming one of these outside a plugin's own source is the host learning a
// plugin's identity, which plugin-host-coupling forbids.
const PLUGIN_SLUG = /['"](?:addon|ai-provider|comm|identity|logging|scanner|snapshot|storage)-[a-z0-9-]+['"]/;

function isHostFrontend(path) {
  return /[\\/]src[\\/]frontend[\\/](?:core|modules|resources)[\\/]/.test(path);
}

/** check runs every TS/TSX pattern against the lines a tool call actually touched. */
export function check({ path, text, changedLines, root = null }) {
  const findings = [];
  // Comments are blanked, not removed, so a pattern quoted in prose ("useTranslation()
  // calls inside ConfigForm…") is not a call site and every line number still holds.
  const lines = blankComments(text).split('\n');
  const checkSlug = isHostFrontend(path);

  lines.forEach((raw, idx) => {
    const line = idx + 1;
    if (!inChanged(changedLines, line)) return;

    if (BARE_NAMESPACE.test(raw)) {
      findings.push({
        line,
        rule: 'i18n/bare-namespace',
        message: "useTranslation() must name a namespace — useTranslation('moduleName'), never bare",
        severity: 'error',
      });
    }

    if (RAW_BUTTON.test(raw)) {
      findings.push({
        line,
        rule: 'ui/raw-button',
        message: 'raw <button> — use <PlainButton> (unstyled) or <Button> from @resources/components/ui/Button',
        severity: 'warn',
      });
    }

    if (INLINE_STYLE.test(raw)) {
      findings.push({
        line,
        rule: 'ui/inline-style',
        message: 'no inline style={{}} — use Tailwind arbitrary values (e.g. min-h-[300px])',
        severity: 'warn',
      });
    }

    if (NATIVE_DIALOG.test(raw)) {
      findings.push({
        line,
        rule: 'ui/native-dialog',
        message: 'window.confirm()/alert() bypass the design system — use <ConfirmDialog> (react-components.md rule 8)',
        severity: 'warn',
      });
    }

    if (DOUBLE_CAST.test(raw)) {
      findings.push({
        line,
        rule: 'ts/double-cast',
        message: 'as unknown as defeats type checking — narrow the type properly or add a type guard',
        severity: 'warn',
      });
    }

    if (RAW_DATE_FORMAT.test(raw)) {
      findings.push({
        line,
        rule: 'i18n/raw-date-format',
        message: 'toLocaleDateString() with no locale ignores the user setting — use formatDate / formatDateCustom from @resources/utils/format',
        severity: 'warn',
      });
    }

    if (QUERY_DROPS_ENVELOPE.test(raw)) {
      findings.push({
        line,
        rule: 'errors/query-drops-envelope',
        message: 'queryFn drops the { success, data } envelope — end it in assertSuccess (@resources/utils/api-mutation) so a failure reaches isError',
        severity: 'error',
      });
    }

    if (GENERIC_ONERROR.test(raw)) {
      findings.push({
        line,
        rule: 'errors/generic-onerror',
        message: 'drop this onError; the global MutationCache toasts the server message; use meta.error to name the operation (error-handling.md rule 5)',
        severity: 'warn',
      });
    }

    if (HARDCODED_COLOR.test(raw)) {
      findings.push({
        line,
        rule: 'ui/hardcoded-color',
        message: 'hardcoded color — use OKLCH theme tokens (bg-card, text-foreground, border-border)',
        severity: 'warn',
      });
    }

    if (checkSlug && PLUGIN_SLUG.test(raw)) {
      findings.push({
        line,
        rule: 'plugins/host-knows-slug',
        message: 'the host must not know a plugin slug — see .claude/rules/plugin-authoring.md and plugin-host-coupling',
        severity: 'error',
      });
    }
  });

  const shim = reexportShimFinding({ path, text, changedLines, root });
  if (shim) findings.push(shim);

  return findings;
}

/* ─── comments ───────────────────────────────────────────────────── */

/**
 * blankComments replaces every comment character with a space, keeping newlines, so
 * offsets and line numbers survive. It tracks quotes and template literals so a `//`
 * inside 'http://…' is not taken for a comment.
 */
export function blankComments(text) {
  const out = text.split('');
  let i = 0;
  let quote = null;
  while (i < out.length) {
    const c = text[i];
    const next = text[i + 1];
    if (quote) {
      if (c === '\\') i += 2;
      else {
        if (c === quote || (c === '\n' && quote !== '`')) quote = null;
        i += 1;
      }
      continue;
    }
    if (c === '/' && next === '/') {
      while (i < out.length && text[i] !== '\n') out[i++] = ' ';
      continue;
    }
    if (c === '/' && next === '*') {
      const end = text.indexOf('*/', i + 2);
      const stop = end < 0 ? out.length : end + 2;
      for (; i < stop; i += 1) if (text[i] !== '\n') out[i] = ' ';
      continue;
    }
    if (c === "'" || c === '"' || c === '`') quote = c;
    i += 1;
  }
  return out.join('');
}

/* ─── code index (shim importers, key references) ────────────────── */

const CODE_ROOTS = ['src/frontend', 'src/plugins/Plugins', 'src/backend', 'widgets', 'nodes'];
const CODE_SKIP = new Set(['node_modules', 'dist', 'build', 'coverage', 'vendor', '.vite', 'locales', 'playwright-report', 'test-results']);
const SEARCH_BUDGET_MS = 5_000;
const indexCache = new Map();

// One read of the code tree per process: the gate and the PostToolUse hook each ask
// several questions of it, and a walk of ~5,000 files costs about 250 ms.
function codeIndex(root) {
  if (indexCache.has(root)) return indexCache.get(root);
  const started = Date.now();
  const files = [];
  let complete = true;
  const walk = (dir) => {
    if (!complete) return;
    let ents;
    try {
      ents = readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of ents) {
      if (Date.now() - started > SEARCH_BUDGET_MS) {
        complete = false;
        return;
      }
      const full = join(dir, e.name);
      if (e.isDirectory()) {
        if (!CODE_SKIP.has(e.name) && !e.name.startsWith('.')) walk(full);
      } else if (/\.(?:tsx?|go|mjs)$/.test(e.name) && !/\.d\.ts$/.test(e.name)) {
        try {
          files.push({ path: full, text: readFileSync(full, 'utf8') });
        } catch {
          // A file deleted mid-walk simply is not a reference.
        }
      }
    }
  };
  for (const r of CODE_ROOTS) walk(join(root, ...r.split('/')));
  const index = { files, complete };
  indexCache.set(root, index);
  return index;
}

/** clearCodeIndex drops the cached tree, for a caller that has just written files into it. */
export function clearCodeIndex() {
  indexCache.clear();
}

/** repoRootOf returns the checkout root above a frontend, plugin, widget or node file. */
function repoRootOf(path) {
  const m = resolve(path).match(/^(.*?)[\\/](?:src[\\/](?:frontend|plugins|backend)|widgets|nodes)[\\/]/);
  return m ? m[1] : null;
}

/* ─── re-export shim ─────────────────────────────────────────────── */

const REEXPORT_ONLY = /^(?:\s*export\s+(?:type\s+)?(?:\*(?:\s+as\s+[\w$]+)?|\{[^}]*\})\s+from\s+(['"])[^'"]+\1\s*;?)+\s*$/;
const SPECIFIER = /(?:\bfrom\s*|\bimport\s*\(\s*|\bimport\s+|\brequire\s*\(\s*|\bvi\.mock\s*\(\s*)(['"`])([^'"`\n]+)\1/g;

/** isReexportOnly reports whether every statement in text is an `export … from` re-export. */
export function isReexportOnly(text) {
  const code = blankComments(text).trim();
  return code !== '' && REEXPORT_ONLY.test(code);
}

function aliasTarget(root, spec) {
  const fe = join(root, 'src', 'frontend');
  const table = [
    ['@resources/', join(fe, 'resources')],
    ['@core/', join(fe, 'core')],
    ['@modules/', join(fe, 'modules')],
    ['@widgets/', join(root, 'widgets')],
    ['@nodes/', join(root, 'nodes')],
    ['@/', fe],
  ];
  if (spec === '@nodes') return join(root, 'nodes');
  for (const [prefix, dir] of table) if (spec.startsWith(prefix)) return join(dir, spec.slice(prefix.length));
  return null;
}

function stripExt(p) {
  return p.replace(/\.(?:tsx?|jsx?|mjs)$/, '');
}

function samePath(a, b) {
  return process.platform === 'win32' ? a.toLowerCase() === b.toLowerCase() : a === b;
}

/**
 * findImporters lists the code files whose import specifiers resolve to file. An
 * alias it cannot resolve counts as an importer, so the answer errs toward "in use".
 */
export function findImporters(root, file) {
  const target = stripExt(resolve(file));
  const isIndex = /[\\/]index$/.test(target);
  const base = isIndex ? target.split(/[\\/]/).slice(-2, -1)[0] : target.split(/[\\/]/).pop();
  const { files, complete } = codeIndex(root);
  const importers = [];
  for (const f of files) {
    if (samePath(resolve(f.path), resolve(file)) || !f.text.includes(base)) continue;
    for (const m of f.text.matchAll(SPECIFIER)) {
      const spec = m[2];
      if (spec.split('/').pop().replace(/\.(?:tsx?|jsx?|mjs)$/, '') !== base) continue;
      let resolved = null;
      if (spec.startsWith('.')) resolved = resolve(dirname(f.path), spec);
      else resolved = aliasTarget(root, spec);
      if (resolved === null) {
        if (spec.startsWith('@')) importers.push(f.path);
        continue;
      }
      const r = stripExt(resolved);
      if (samePath(r, target) || (isIndex && samePath(join(r, 'index'), target))) {
        importers.push(f.path);
        break;
      }
    }
  }
  return { importers, complete };
}

function reexportShimFinding({ path, text, changedLines, root }) {
  if (!/\.tsx?$/.test(path) || !isReexportOnly(text)) return null;
  const lines = text.split('\n');
  const first = lines.findIndex((l) => /^\s*export\b/.test(l));
  if (first < 0) return null;
  const touched = lines.some((_, i) => inChanged(changedLines, i + 1));
  if (!touched) return null;
  const base = root ?? repoRootOf(path);
  if (!base) return null;
  const { importers, complete } = findImporters(base, path);
  if (!complete || importers.length) return null;
  return {
    line: first + 1,
    rule: 'ts/reexport-shim',
    message: 're-export shim with no importer — delete it and import the real module (module-boundaries.md rule 7)',
    severity: 'error',
  };
}

/* ─── removed translation keys ───────────────────────────────────── */

const T_LITERAL = /(?<![\w$])(?:t|t[A-Z][\w$]*|i18n\.t)\(\s*(['"])([^'"\n]+)\1/g;
const KEY_SHAPE = /^[A-Za-z0-9_][\w-]*(?:\.[\w-]+)+$/;

/** literalKeys returns the dotted keys of every literal t('…') call, without an `ns:` prefix. */
export function literalKeys(text) {
  const keys = new Set();
  for (const m of blankComments(text).matchAll(T_LITERAL)) {
    const raw = m[2];
    const key = raw.includes(':') ? raw.slice(raw.indexOf(':') + 1) : raw;
    if (KEY_SHAPE.test(key)) keys.add(key);
  }
  return keys;
}

/** removedKeys returns the literal keys before carries that after no longer does. */
export function removedKeys(before, after) {
  const now = literalKeys(after);
  return new Set([...literalKeys(before)].filter((k) => !now.has(k)));
}

const QUOTES = new Set(["'", '"', '`']);

function referenced(text, key) {
  let at = text.indexOf(key);
  while (at >= 0) {
    const prev = text[at - 1];
    const next = text[at + key.length];
    if ((QUOTES.has(prev) || prev === ':') && QUOTES.has(next)) return true;
    at = text.indexOf(key, at + 1);
  }
  return false;
}

/**
 * unreferencedKeys returns the keys no code file quotes anywhere — Go included, since
 * the backend sends some keys as reason codes. `complete` is false when the walk ran
 * out of budget, and then nothing is reported.
 */
export function unreferencedKeys(root, keys) {
  const { files, complete } = codeIndex(root);
  if (!complete) return { unused: [], complete };
  const unused = [...keys].filter((k) => !files.some((f) => referenced(f.text, k))).sort();
  return { unused, complete };
}

/** keyUnusedFindings turns removed-and-unreferenced keys into warnings on line 1. */
export function keyUnusedFindings(root, before, after) {
  const removed = removedKeys(before, after);
  if (!removed.size || !root) return [];
  const { unused } = unreferencedKeys(root, removed);
  return unused.map((key) => ({
    line: 1,
    rule: 'i18n/key-unused',
    message: `t('${key}') was removed and nothing else references the key — delete it from all six locales (i18n check 7)`,
    severity: 'warn',
  }));
}

/** rootFor exposes the checkout-root heuristic to the hooks that call keyUnusedFindings. */
export function rootFor(path) {
  return repoRootOf(path);
}
