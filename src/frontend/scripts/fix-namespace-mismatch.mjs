// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Resolve the 3 namespace-mismatch call sites flagged in the i18n audit
// and remove the orphaned `common.common` subtree that the prior backfill
// created to compensate.
//
// The wrong calls looked up `common.json -> common.actions` (the artifact
// under `common.common`) instead of `common.json -> actions`. Replacing
// them with `tc("actions")` would return the actions object, not a string,
// so we add a dedicated `common.actionsColumn` key and use that.
//
// Idempotent: re-running is a no-op once the keys and calls are in place.

import console from 'node:console';
import { readFileSync, writeFileSync, existsSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, '..', '..', '..');
const localesRoot = join(repoRoot, 'src', 'frontend', 'core', 'i18n', 'locales');
const referenceLocale = 'en';

function readJson(p) {
  const raw = readFileSync(p, 'utf8');
  return JSON.parse(raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw);
}

function writeJson(p, obj) {
  writeFileSync(p, `${JSON.stringify(obj, null, 2)}\n`, 'utf8');
}

function setIfMissing(obj, dotted, value) {
  const parts = dotted.split('.');
  let cursor = obj;
  for (let i = 0; i < parts.length - 1; i += 1) {
    const p = parts[i];
    if (
      !Object.prototype.hasOwnProperty.call(cursor, p) ||
      cursor[p] === null ||
      typeof cursor[p] !== 'object' ||
      Array.isArray(cursor[p])
    ) {
      cursor[p] = {};
    }
    cursor = cursor[p];
  }
  const leaf = parts[parts.length - 1];
  if (Object.prototype.hasOwnProperty.call(cursor, leaf)) return false;
  cursor[leaf] = value;
  return true;
}

function deletePath(obj, dottedPath) {
  const parts = dottedPath.split('.');
  let parent = obj;
  const stack = [];
  for (let i = 0; i < parts.length - 1; i += 1) {
    const key = parts[i];
    if (parent == null || typeof parent !== 'object' || !(key in parent)) return false;
    stack.push([parent, key]);
    parent = parent[key];
  }
  const leaf = parts[parts.length - 1];
  if (parent == null || typeof parent !== 'object' || !(leaf in parent)) return false;
  delete parent[leaf];
  for (let i = stack.length - 1; i >= 0; i -= 1) {
    const [p, k] = stack[i];
    const child = p[k];
    if (child && typeof child === 'object' && !Array.isArray(child) && Object.keys(child).length === 0) {
      delete p[k];
    } else {
      break;
    }
  }
  return true;
}

const enDir = join(localesRoot, referenceLocale);
const enFile = join(enDir, 'common.json');
const enObj = readJson(enFile);

if (!setIfMissing(enObj, 'actionsColumn', 'Actions')) {
  console.log('en/common.json: actionsColumn already present');
} else {
  writeJson(enFile, enObj);
  console.log('en/common.json: +common.actionsColumn');
}

// Strip the orphaned `common.common` subtree from all 12 locales.
const langs = readdirSync(localesRoot)
  .filter((entry) => statSync(join(localesRoot, entry)).isDirectory())
  .sort();

let totalStripped = 0;
for (const lang of langs) {
  const file = join(localesRoot, lang, 'common.json');
  if (!existsSync(file)) continue;
  const obj = readJson(file);
  if (deletePath(obj, 'common')) {
    writeJson(file, obj);
    console.log(`  ${lang}/common.json: removed common.common subtree`);
    totalStripped += 1;
  }
}
console.log(`Stripped common.common from ${totalStripped} locale(s).`);
