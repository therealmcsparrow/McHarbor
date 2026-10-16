// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// Backfill en locale with leaf placeholders for every key missing from
// namespaces that t() calls resolve against, and propagate en placeholders
// to the other 11 locales so the parity gate (check-i18n.mjs) stays green.
// Run after i18n:check-keys surfaces misses. Existing translations are
// never overwritten; new leaves get a "{{TODO: translate}}" placeholder so
// the next human pass can find them with grep. Idempotent.
//
// Strategy:
//   1. Spawn check-t-resolves.mjs, parse "MISSING <file>:<line>  key='X'
//      ns='A|B'" lines. Primary namespace is the first pipe-separated
//      token (i18next's lookup order in the array form).
//   2. Set each missing dotted path as a leaf in en/<ns>.json, building
//      nested objects as needed.
//   3. Run a full en → other-locales parity sync so the parity gate sees
//      every key en has. Existing non-en translations are never
//      overwritten; only missing keys get filled.
//
// Matches the pattern in scripts/backfill-co-located-i18n.mjs for
// widgets/nodes.

import console from 'node:console';
import process from 'node:process';
import { spawnSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const localesRoot = join(scriptDir, '..', 'core', 'i18n', 'locales');
const enDir = join(localesRoot, 'en');

const PLACEHOLDER = '{{TODO: translate}}';

function collectMisses() {
  const res = spawnSync(process.execPath, [join(scriptDir, 'check-t-resolves.mjs')], {
    encoding: 'utf8',
  });
  const lines = (res.stderr || '').split(/\r?\n/);
  const misses = [];
  for (const line of lines) {
    const m = /^MISSING\s+(\S+):(\d+)\s+key='([^']+)'\s+ns='([^']+)'/.exec(line);
    if (!m) continue;
    misses.push({
      file: m[1],
      line: Number(m[2]),
      key: m[3],
      primaryNs: m[4].split('|')[0],
    });
  }
  return misses;
}

// Set leaves in `obj` at `dotted` without disturbing any existing
// value at the same path. Returns true if a new leaf was added.
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

// Resolve a dotted path against a parsed JSON tree. Returns the
// string at that path, or undefined if any segment is missing or
// descends into a non-object / non-leaf.
function getAtPath(obj, dotted) {
  let cursor = obj;
  for (const p of dotted.split('.')) {
    if (cursor === null || typeof cursor !== 'object') return undefined;
    cursor = cursor[p];
  }
  return typeof cursor === 'string' ? cursor : undefined;
}

// Walk `obj` and collect every leaf as a dotted path.
function collectLeafKeys(obj, prefix = '', out = new Set()) {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    if (prefix) out.add(prefix);
    return out;
  }
  for (const [k, v] of Object.entries(obj)) {
    const next = prefix ? `${prefix}.${k}` : k;
    collectLeafKeys(v, next, out);
  }
  return out;
}

function readLocaleObj(file) {
  if (!existsSync(file)) return { obj: {}, isNew: true, hasBom: false };
  const text = readFileSync(file, 'utf8');
  const hasBom = text.charCodeAt(0) === 0xfeff;
  const stripped = hasBom ? text.slice(1) : text;
  return { obj: JSON.parse(stripped), isNew: false, hasBom };
}

function writeLocaleObj(file, obj, hasBom) {
  const out = JSON.stringify(obj, null, 4);
  const body = hasBom ? '﻿' + out : out;
  writeFileSync(file, body);
}

function ensureKeys(file, keys, valueSource) {
  const { obj, isNew, hasBom } = readLocaleObj(file);
  let addedHere = 0;
  for (const k of keys) {
    const value = getAtPath(valueSource, k) ?? PLACEHOLDER;
    if (setIfMissing(obj, k, value)) addedHere += 1;
  }
  if (addedHere > 0 || isNew) {
    writeLocaleObj(file, obj, hasBom);
    return { added: addedHere, created: isNew };
  }
  return { added: 0, created: false };
}

function main() {
  const langs = readdirSync(localesRoot).filter((entry) => entry !== 'en' && existsSync(join(localesRoot, entry)));

  // Pass 1 — fill en with TODO placeholders for every missing key.
  const misses = collectMisses();
  if (misses.length > 0) {
    const byNs = new Map();
    for (const m of misses) {
      if (!byNs.has(m.primaryNs)) byNs.set(m.primaryNs, new Set());
      byNs.get(m.primaryNs).add(m.key);
    }
    console.log(`backfill: ${misses.length} missing-key call site(s) across ${byNs.size} namespace(s)`);
    let totalAdded = 0;
    for (const [ns, keys] of byNs) {
      const { added, created } = ensureKeys(join(enDir, `${ns}.json`), keys, {});
      if (created) console.log(`  ${ns}: created file +${added} key(s)`);
      else if (added > 0) console.log(`  ${ns}: +${added} new keys`);
      totalAdded += added;
    }
    console.log(`backfill: added ${totalAdded} TODO placeholder(s) in en; existing translations preserved`);
  } else {
    console.log('backfill: no missing keys in en');
  }

  // Pass 2 — full parity sync. For every en/<ns>.json, ensure every
  // other locale's <ns>.json has the same leaf set; fill missing leaves
  // with the en value (typically the TODO placeholder).
  console.log('backfill: parity sync en → 11 other locales');
  let parityAdded = 0;
  for (const file of readdirSync(enDir).filter((f) => f.endsWith('.json'))) {
    const ns = file.replace(/\.json$/, '');
    const enFile = join(enDir, file);
    const { obj: enObj } = readLocaleObj(enFile);
    const enKeys = collectLeafKeys(enObj);
    for (const lang of langs) {
      const tgt = join(localesRoot, lang, file);
      const { added } = ensureKeys(tgt, enKeys, enObj);
      if (added > 0) {
        parityAdded += added;
        console.log(`  ${lang}/${ns}: +${added} mirrored`);
      }
    }
  }
  if (parityAdded > 0) console.log(`backfill: parity sync added ${parityAdded} key(s)`);
  else console.log('backfill: parity sync clean');
}

main();
