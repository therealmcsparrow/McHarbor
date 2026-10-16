// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Locale parity fixer for sanity check i18n #11.
//
// Every key that exists in locales/en/<ns>.json must exist in all five other
// locales. The repo convention is that an untranslated key carries the English
// string verbatim, so the bundles stay key-identical and a genuine miss stays
// detectable. This script copies missing keys from en into the other locales,
// preserving en's nesting and key order, and never touches a key a locale has
// already translated.
//
// Usage:
//   node scripts/sanity/i18n/sync-locales.mjs           # report only (exit 1 if drift)
//   node scripts/sanity/i18n/sync-locales.mjs --write   # write the missing keys

import fs from 'node:fs';
import path from 'node:path';

const ROOT = path.resolve(process.cwd(), 'src/frontend/core/i18n/locales');
const LOCALES = ['nl', 'de', 'es', 'fr', 'pt'];
const WRITE = process.argv.includes('--write');

const isObj = (v) => v !== null && typeof v === 'object' && !Array.isArray(v);

/** Merge en into target, returning [merged, missingKeyPaths]. */
function merge(en, target, prefix = '') {
  const out = {};
  const missing = [];
  for (const [key, enVal] of Object.entries(en)) {
    const keyPath = prefix ? `${prefix}.${key}` : key;
    const has = Object.prototype.hasOwnProperty.call(target ?? {}, key);
    if (isObj(enVal)) {
      const [child, childMissing] = merge(enVal, has && isObj(target[key]) ? target[key] : {}, keyPath);
      out[key] = child;
      missing.push(...childMissing);
    } else if (has) {
      out[key] = target[key];
    } else {
      out[key] = enVal;
      missing.push(keyPath);
    }
  }
  // Keep keys the locale has that en does not; check 11 does not flag them,
  // and silently dropping a translation would be data loss.
  for (const [key, val] of Object.entries(target ?? {})) {
    if (!Object.prototype.hasOwnProperty.call(out, key)) out[key] = val;
  }
  return [out, missing];
}

const namespaces = fs
  .readdirSync(path.join(ROOT, 'en'))
  .filter((f) => f.endsWith('.json'));

let total = 0;
const changed = [];

for (const ns of namespaces) {
  const en = JSON.parse(fs.readFileSync(path.join(ROOT, 'en', ns), 'utf8'));
  for (const locale of LOCALES) {
    const file = path.join(ROOT, locale, ns);
    if (!fs.existsSync(file)) {
      console.error(`MISSING FILE ${locale}/${ns} - create it before syncing`);
      total += 1;
      continue;
    }
    const target = JSON.parse(fs.readFileSync(file, 'utf8'));
    const [merged, missing] = merge(en, target);
    if (missing.length === 0) continue;
    total += missing.length;
    changed.push(`${locale}/${ns} (${missing.length})`);
    for (const key of missing) console.log(`${locale}/${ns} - ${key}`);
    if (WRITE) {
      // LF only: this repo is LF on disk and a CRLF rewrite shows up as a
      // whole-file diff.
      fs.writeFileSync(file, `${JSON.stringify(merged, null, 2)}\n`.replace(/\r\n/g, '\n'), 'utf8');
    }
  }
}

if (total === 0) {
  console.log('Locale parity OK - every en key exists in nl, de, es, fr, pt.');
  process.exit(0);
}

console.log(`\n${total} missing key(s) across: ${changed.join(', ')}`);
if (WRITE) {
  console.log('Written. English text copied verbatim - translate when convenient.');
  process.exit(0);
}
console.log('Run with --write to copy the English values in.');
process.exit(1);
