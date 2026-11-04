// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync, readdirSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { pathToFileURL } from 'node:url';

// Mirrors validateCatalogEntry and parseCatalogIndex in
// src/plugins/Plugins/addon-compute/backend/catalog.go. The plugin skips a bad
// entry rather than failing the index, so a mistake here would publish silently
// and only surface as a skipped line in someone's reload result.
export const MAX_INDEX_BYTES = 4 << 20;
export const MAX_ENTRIES = 500;
export const MAX_ENTRY_BYTES = 64 * 1024;
const KINDS = new Set(['cloud_image', 'rootfs', 'iso']);
const ALGOS = new Set(['sha256', 'sha512']);
const FIELDS = new Set([
  'id', 'kind', 'distro', 'release', 'arch', 'variant', 'displayName', 'sourceUrl',
  'checksumUrl', 'checksum', 'checksumAlgo', 'signatureUrl', 'sizeBytes',
  'initSystem', 'netConfigFormat',
]);
const ID_RE = /^[a-z0-9._-]+$/;

export const INDEX_FILE = 'index.json';

export function entryFiles(dir) {
  return readdirSync(dir)
    .filter((f) => f.toLowerCase().endsWith('.json') && f.toLowerCase() !== INDEX_FILE)
    .sort();
}

function validate(file, entry) {
  if (!entry || typeof entry !== 'object' || Array.isArray(entry)) throw new Error(`${file}: not a JSON object`);
  for (const key of Object.keys(entry)) {
    if (!FIELDS.has(key)) throw new Error(`${file}: unknown field "${key}"`);
  }
  if (typeof entry.id !== 'string' || !ID_RE.test(entry.id.trim())) throw new Error(`${file}: invalid id`);
  if (!KINDS.has(entry.kind)) throw new Error(`${file}: kind must be cloud_image, rootfs or iso`);
  for (const key of ['distro', 'release', 'arch', 'displayName']) {
    if (typeof entry[key] !== 'string' || entry[key].trim() === '') throw new Error(`${file}: ${key} is required`);
  }
  for (const key of ['sourceUrl', 'checksumUrl', 'signatureUrl']) {
    const value = entry[key];
    if (value === undefined || (key !== 'sourceUrl' && value === '')) continue;
    if (typeof value !== 'string' || !value.startsWith('https://')) throw new Error(`${file}: ${key} must be an https URL`);
  }
  if (entry.sourceUrl === undefined) throw new Error(`${file}: sourceUrl must be an https URL`);
  if (entry.checksumAlgo !== undefined && !ALGOS.has(entry.checksumAlgo)) throw new Error(`${file}: checksumAlgo must be sha256 or sha512`);
  if (entry.sizeBytes !== undefined && (!Number.isInteger(entry.sizeBytes) || entry.sizeBytes < 0)) {
    throw new Error(`${file}: sizeBytes must be a non-negative integer`);
  }
}

export function buildIndex(dir) {
  const files = entryFiles(dir);
  if (files.length === 0) throw new Error(`${dir} holds no catalog entries`);
  if (files.length > MAX_ENTRIES) throw new Error(`${files.length} entries exceeds the ${MAX_ENTRIES} limit`);

  const seen = new Map();
  const unverified = [];
  const images = files.map((file) => {
    const raw = readFileSync(join(dir, file), 'utf8');
    if (Buffer.byteLength(raw) > MAX_ENTRY_BYTES) throw new Error(`${file}: exceeds ${MAX_ENTRY_BYTES} bytes`);
    let entry;
    try {
      entry = JSON.parse(raw);
    } catch (err) {
      throw new Error(`${file}: ${err.message}`);
    }
    validate(file, entry);
    if (seen.has(entry.id)) throw new Error(`${file}: id "${entry.id}" already defined by ${seen.get(entry.id)}`);
    seen.set(entry.id, file);
    if (!entry.checksumUrl && !entry.checksum) unverified.push(entry.id);
    return entry;
  });

  const text = `${JSON.stringify({ version: '1.0.0', images }, null, 2)}\n`;
  const bytes = Buffer.byteLength(text);
  if (bytes > MAX_INDEX_BYTES) throw new Error(`index is ${bytes} bytes, over the ${MAX_INDEX_BYTES} limit`);
  return { text, count: images.length, bytes, unverified };
}

function main(argv) {
  const check = argv.includes('--check');
  const positional = argv.filter((a) => !a.startsWith('--'));
  // mnt/compute-catalog is the staging folder committed as-is to the Images/ folder
  // of McHarbor-Catalog, so the index lands beside its entries.
  const dir = positional[0] ?? 'mnt/compute-catalog';
  const target = positional[1] ?? join(dir, INDEX_FILE);
  const { text, count, bytes, unverified } = buildIndex(dir);

  if (check) {
    const current = existsSync(target) ? readFileSync(target, 'utf8') : '';
    if (current !== text) {
      console.error(`${target} is out of date; run: node scripts/compute/build-image-index.mjs ${dir} ${target}`);
      process.exit(1);
    }
    console.log(`${target} is current (${count} images)`);
    return;
  }

  mkdirSync(dirname(target), { recursive: true });
  writeFileSync(target, text);
  console.log(`wrote ${target}: ${count} images, ${(bytes / 1024).toFixed(1)} KiB`);
  if (unverified.length > 0) console.log(`no checksum (downloads unverified): ${unverified.join(', ')}`);
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  try {
    main(process.argv.slice(2));
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }
}
