// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { collectPluginRoutes } from '../../../scripts/sanity/missing-api/plugin-routes.mjs';

const REPO = resolve(__dirname, '..', '..', '..');
const OPENAPI_PATH = join(REPO, 'src/backend/api/openapi.json');
const PLUGIN_ROUTES_PATH = resolve(__dirname, 'fixtures', 'plugin-routes.json');
const ALLOWLIST_PATH = resolve(__dirname, 'fixtures', 'api-paths-allowlist.json');
const SCAN_ROOTS = ['core', 'modules', 'resources'].map((dir) => join(REPO, 'src/frontend', dir));
const SKIP_DIRS = new Set(['node_modules', 'dist', 'build', 'coverage']);

type Allowed = { call: string; reason: string };
type CallSite = { call: string; method: string; path: string };
type KnownRoute = { method: string; path: string };

const METHODS = ['get', 'post', 'put', 'patch', 'del'] as const;
const METHOD_MAP: Record<string, string> = { get: 'GET', post: 'POST', put: 'PUT', patch: 'PATCH', del: 'DELETE' };

function walk(dir: string, out: string[]) {
  if (!existsSync(dir)) return;
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      walk(path, out);
    } else if (/\.(ts|tsx)$/.test(path) && !/\.(test|spec)\.tsx?$/.test(path)) {
      out.push(path);
    }
  }
}

/**
 * skipStringLiteral advances past a '..' or "..." literal starting at i
 * (source[i] is the opening quote), returning the index right after the
 * closing quote. It exists so scanTemplateBody can step over a quoted string
 * inside a `${...}` interpolation without being fooled by a brace or a
 * backtick the string happens to contain.
 */
function skipStringLiteral(source: string, i: number): number {
  const quote = source[i];
  i++;
  while (i < source.length && source[i] !== quote) {
    i += source[i] === '\\' ? 2 : 1;
  }
  return i + 1;
}

/**
 * scanTemplateBody reads a template literal body starting right after its
 * opening backtick, returning the raw text up to (not including) the closing
 * backtick and the index right after that backtick. It tracks `${...}`
 * nesting depth so a nested template literal's own backticks — the shape
 * every `cond ? \`a\` : \`b\`` interpolation uses — never terminate the scan
 * early, which a single regex cannot do.
 */
function scanTemplateBody(source: string, start: number): { raw: string; end: number } {
  let i = start;
  let depth = 0;
  while (i < source.length) {
    const c = source[i];
    if (c === '\\') {
      i += 2;
      continue;
    }
    if (depth === 0 && c === '`') {
      return { raw: source.slice(start, i), end: i + 1 };
    }
    if (depth === 0 && c === '$' && source[i + 1] === '{') {
      depth = 1;
      i += 2;
      continue;
    }
    if (depth > 0) {
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '`') {
        i = scanTemplateBody(source, i + 1).end;
        continue;
      } else if (c === '"' || c === "'") {
        i = skipStringLiteral(source, i);
        continue;
      }
    }
    i++;
  }
  return { raw: source.slice(start), end: source.length };
}

/**
 * scanInterpolationEnd returns the index right after the `}` that closes a
 * `${...}` interpolation, given the index of the first character after `${`.
 * It tracks brace depth and steps over nested strings/templates so a brace
 * inside one of those never closes the interpolation early.
 */
function scanInterpolationEnd(source: string, start: number): number {
  let i = start;
  let depth = 1;
  while (i < source.length && depth > 0) {
    const c = source[i];
    if (c === '\\') {
      i += 2;
      continue;
    }
    if (c === '{') {
      depth++;
      i++;
    } else if (c === '}') {
      depth--;
      i++;
    } else if (c === '`') {
      i = scanTemplateBody(source, i + 1).end;
    } else if (c === '"' || c === "'") {
      i = skipStringLiteral(source, i);
    } else {
      i++;
    }
  }
  return i;
}

/** normalizeTemplate collapses every top-level `${...}` block in raw to a single "{param}" token. */
function normalizeTemplate(raw: string): string {
  let out = '';
  let i = 0;
  while (i < raw.length) {
    if (raw[i] === '\\') {
      out += raw[i] + (raw[i + 1] ?? '');
      i += 2;
      continue;
    }
    if (raw[i] === '$' && raw[i + 1] === '{') {
      i = scanInterpolationEnd(raw, i + 2);
      out += '{param}';
      continue;
    }
    out += raw[i];
    i++;
  }
  return out;
}

/**
 * tidyPath strips a literal query string and collapses a trailing segment
 * that glues a query-string interpolation onto a real path segment — the
 * `` `/networks${envQuery}` `` and `` `/containers/${id}/rename${qs ? ... :
 * ''}` `` shapes, where the trailing `{param}` is a `?env=...` suffix rather
 * than a distinct path segment.
 */
function tidyPath(path: string): string {
  const queryIndex = path.indexOf('?');
  let clean = queryIndex === -1 ? path : path.slice(0, queryIndex);
  const segments = clean.split('/');
  const last = segments[segments.length - 1] ?? '';
  if (last !== '{param}' && last.endsWith('{param}')) {
    segments[segments.length - 1] = last.slice(0, -'{param}'.length);
  }
  clean = segments.join('/');
  return clean.replace(/\/+$/, '') || '/';
}

function scanCallSites(): CallSite[] {
  const files: string[] = [];
  for (const root of SCAN_ROOTS) walk(root, files);

  const sites: CallSite[] = [];
  for (const file of files) {
    const rel = relative(REPO, file).replace(/\\/g, '/');
    const source = readFileSync(file, 'utf8');
    const callRe = new RegExp(`\\bapi\\.(${METHODS.join('|')})\\s*(?:<[^(]*>)?\\s*\\(`, 'g');
    for (const match of source.matchAll(callRe)) {
      const method = METHOD_MAP[match[1] ?? ''];
      if (!method) continue;
      let i = match.index! + match[0].length;
      while (source[i] === ' ' || source[i] === '\n' || source[i] === '\t') i++;
      const quote = source[i];
      let rawPath: string | undefined;
      if (quote === '`') {
        rawPath = normalizeTemplate(scanTemplateBody(source, i + 1).raw);
      } else if (quote === '"' || quote === "'") {
        const end = skipStringLiteral(source, i);
        rawPath = source.slice(i + 1, end - 1);
      }
      if (!rawPath || !rawPath.startsWith('/')) continue; // dynamic/derived, not a literal API path
      const path = `/api${tidyPath(rawPath)}`;
      sites.push({ call: `${rel}: ${method} ${path.slice(4)}`, method, path });
    }
  }
  return sites;
}

function pathShape(path: string): string[] {
  return path
    .split('/')
    .filter(Boolean)
    .map((seg) => (seg.startsWith('{') && seg.endsWith('}') ? '{}' : seg));
}

/**
 * shapesMatch compares a frontend call site's path segments (call) against an
 * openapi route's (api). It is deliberately asymmetric: an openapi `{param}`
 * segment accepts anything the frontend sends there (a literal or its own
 * `{param}`), because the backend genuinely takes an arbitrary value. An
 * openapi *literal* segment, though, only accepts the identical literal from
 * the frontend — a frontend `{param}` there means the call site interpolates
 * a runtime-chosen action name (start/stop/rename/…) into a position the
 * backend does not templatize, so which literal endpoint actually gets hit
 * cannot be verified statically and the call site needs a justified
 * allow-list entry instead of a silent pass.
 */
function shapesMatch(call: string[], api: string[]): boolean {
  if (call.length !== api.length) return false;
  return call.every((seg, i) => api[i] === '{}' || seg === api[i]);
}

function loadKnownRoutes(): KnownRoute[] {
  const openapi = JSON.parse(readFileSync(OPENAPI_PATH, 'utf8')) as { paths: Record<string, Record<string, unknown>> };
  const known: KnownRoute[] = [];
  for (const [path, methods] of Object.entries(openapi.paths)) {
    for (const method of Object.keys(methods)) {
      if (method === 'parameters') continue;
      known.push({ method: method.toUpperCase(), path });
    }
  }
  if (existsSync(PLUGIN_ROUTES_PATH)) {
    const plugin = JSON.parse(readFileSync(PLUGIN_ROUTES_PATH, 'utf8')) as { method: string; path: string }[];
    for (const route of plugin) known.push({ method: route.method, path: route.path });
  }
  return known;
}

function resolves(site: CallSite, known: KnownRoute[]): boolean {
  const shape = pathShape(site.path);
  return known.some((route) => route.method === site.method && shapesMatch(shape, pathShape(route.path)));
}

const allowList = JSON.parse(readFileSync(ALLOWLIST_PATH, 'utf8')) as Allowed[];

describe('frontend API paths resolve against openapi.json + plugin routes', () => {
  const known = loadKnownRoutes();
  const sites = scanCallSites();
  expect(known.length, 'openapi.json + plugin-routes.json look truncated').toBeGreaterThan(400);
  expect(sites.length, 'the api.(get|post|put|patch|del) scan found suspiciously few call sites').toBeGreaterThan(200);

  const unresolved = sites.filter((site) => !resolves(site, known));
  const allowed = new Map(allowList.map((entry) => [entry.call, entry.reason]));

  it('resolves every literal frontend API call outside the allow-list', () => {
    const offenders = unresolved.filter((site) => !allowed.has(site.call)).map((site) => site.call);
    expect(offenders, 'add the missing backend route, or a justified allow-list entry').toEqual([]);
  });

  it('keeps every allow-list entry justified and still unresolved', () => {
    const unresolvedCalls = new Set(unresolved.map((site) => site.call));
    for (const [call, reason] of allowed) {
      expect(reason.trim().length, `${call} needs a reason`).toBeGreaterThan(10);
      expect(unresolvedCalls.has(call), `${call} now resolves; drop it from the allow-list`).toBe(true);
    }
  });

  it('keeps plugin-routes.json current with the plugin.yaml files, when the plugin tree is present', () => {
    const pluginTree = join(REPO, 'src/plugins/Plugins');
    if (!existsSync(pluginTree)) return; // nothing to compare the checked-in fixture against

    const live = collectPluginRoutes(pluginTree);
    const checkedIn = JSON.parse(readFileSync(PLUGIN_ROUTES_PATH, 'utf8')) as unknown[];
    expect(
      live,
      'plugin-routes.json is stale; run `node scripts/sanity/missing-api/plugin-routes.mjs` to regenerate it',
    ).toEqual(checkedIn);
  });
});
