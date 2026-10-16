// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync, readdirSync, writeFileSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const REPO = resolve(fileURLToPath(import.meta.url), '..', '..', '..', '..');
const PLUGINS_ROOT = join(REPO, 'src', 'plugins', 'Plugins');
const OUT_FILE = join(REPO, 'src', 'backend', 'core', 'rbac', 'testdata', 'plugin-permissions.json');

const PERMISSIONS_KEY = /^adds_permissions:\s*$/;
const KEY_LINE = /^ {2}- key:\s+"([^"]+)"\s*$/;

export function permissionsBlockLines(manifestText) {
  const lines = manifestText.split(/\r?\n/);
  const start = lines.findIndex((line) => PERMISSIONS_KEY.test(line));
  if (start === -1) return [];
  const block = [];
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === '') continue;
    if (!line.startsWith('  ')) break;
    block.push(line);
  }
  return block;
}

export function extractPermissionKeys(manifestText) {
  const lines = permissionsBlockLines(manifestText);
  const keys = [];
  for (const line of lines) {
    const match = KEY_LINE.exec(line);
    if (match) keys.push(match[1]);
  }
  return keys;
}

export function collectPluginPermissions(pluginsRoot = PLUGINS_ROOT) {
  if (!existsSync(pluginsRoot)) return [];
  const slugs = readdirSync(pluginsRoot, { withFileTypes: true })
    .filter((e) => e.isDirectory() && !e.name.startsWith('_'))
    .map((e) => e.name)
    .sort();

  const keys = new Set();
  for (const slug of slugs) {
    const manifestPath = join(pluginsRoot, slug, 'plugin.yaml');
    if (!existsSync(manifestPath)) continue;
    const text = readFileSync(manifestPath, 'utf8');
    for (const key of extractPermissionKeys(text)) keys.add(key);
  }
  return [...keys].sort();
}

function main() {
  const keys = collectPluginPermissions();
  const json = JSON.stringify(keys, null, 2) + '\n';

  if (process.argv.includes('--check')) {
    if (!existsSync(OUT_FILE)) {
      console.error(`${OUT_FILE} does not exist; run without --check to generate it`);
      process.exitCode = 1;
      return;
    }
    const current = readFileSync(OUT_FILE, 'utf8');
    if (current !== json) {
      console.error(`${OUT_FILE} is stale; run node scripts/sanity/missing-api/plugin-permissions.mjs to regenerate it`);
      process.exitCode = 1;
      return;
    }
    console.log(`${OUT_FILE} is current (${keys.length} keys)`);
    return;
  }

  writeFileSync(OUT_FILE, json);
  console.log(`wrote ${keys.length} plugin permission keys from ${PLUGINS_ROOT} to ${OUT_FILE}`);
}

const isMain = process.argv[1] && pathToFileURL(process.argv[1]).href === import.meta.url;
if (isMain) main();
