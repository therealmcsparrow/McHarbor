// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync, readdirSync, writeFileSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const REPO = resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..');
const PLUGINS_ROOT = join(REPO, 'src', 'plugins', 'Plugins');
const OUT_FILE = join(REPO, 'src', 'frontend', 'test', 'fixtures', 'plugin-routes.json');

const ROUTES_KEY = /^ {2}routes:\s*$/;
const METHOD_LINE = /^ {4}- method:\s+(\S+)\s*$/;
const PATH_LINE = /^ {6}path:\s+"([^"]+)"\s*$/;
const PUBLIC_LINE = /^ {6}public:\s+true\s*$/;

export function routesBlockLines(manifestText) {
  const lines = manifestText.split(/\r?\n/);
  const start = lines.findIndex((line) => ROUTES_KEY.test(line));
  if (start === -1) return [];
  const block = [];
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === '') continue;
    if (!line.startsWith('    ')) break;
    block.push(line);
  }
  return block;
}

export function extractRoutes(manifestText, manifestPath) {
  const lines = routesBlockLines(manifestText);
  const routes = [];
  let current = null;
  let currentIndex = -1;

  const closeCurrent = () => {
    if (!current) return;
    if (!current.path) {
      throw new Error(`${manifestPath}: routes entry #${currentIndex} (method ${current.method}) has no path line`);
    }
    routes.push(current);
  };

  for (const line of lines) {
    const method = METHOD_LINE.exec(line);
    if (method) {
      closeCurrent();
      currentIndex++;
      current = { method: method[1].toUpperCase(), path: '', public: false };
      continue;
    }
    if (!current) continue;
    const path = PATH_LINE.exec(line);
    if (path) {
      current.path = path[1];
      continue;
    }
    if (PUBLIC_LINE.test(line)) {
      current.public = true;
    }
  }
  closeCurrent();
  return routes;
}

export function collectPluginRoutes(pluginsRoot = PLUGINS_ROOT) {
  if (!existsSync(pluginsRoot)) return [];
  const slugs = readdirSync(pluginsRoot, { withFileTypes: true })
    .filter((e) => e.isDirectory() && !e.name.startsWith('_'))
    .map((e) => e.name)
    .sort();

  const out = [];
  for (const slug of slugs) {
    const manifestPath = join(pluginsRoot, slug, 'plugin.yaml');
    if (!existsSync(manifestPath)) continue;
    const text = readFileSync(manifestPath, 'utf8');
    for (const route of extractRoutes(text, manifestPath)) {
      out.push({ slug, ...route });
    }
  }
  return out;
}

function main() {
  const routes = collectPluginRoutes();
  const json = JSON.stringify(routes, null, 2) + '\n';

  if (process.argv.includes('--check')) {
    if (!existsSync(OUT_FILE)) {
      console.error(`${OUT_FILE} does not exist; run without --check to generate it`);
      process.exitCode = 1;
      return;
    }
    const current = readFileSync(OUT_FILE, 'utf8');
    if (current !== json) {
      console.error(`${OUT_FILE} is stale; run node scripts/sanity/missing-api/plugin-routes.mjs to regenerate it`);
      process.exitCode = 1;
      return;
    }
    console.log(`${OUT_FILE} is current (${routes.length} routes)`);
    return;
  }

  writeFileSync(OUT_FILE, json);
  console.log(`wrote ${routes.length} plugin routes from ${PLUGINS_ROOT} to ${OUT_FILE}`);
}

const isMain = process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url;
if (isMain) main();
