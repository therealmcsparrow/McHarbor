import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const baseDir = path.resolve('core/i18n/locales');
const referenceLang = 'en';
const coLocatedI18nRoots = [path.resolve('widgets'), path.resolve('nodes')];

function flattenKeys(value, prefix = '') {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return [prefix];
  }

  return Object.entries(value).flatMap(([key, nested]) => {
    const next = prefix ? `${prefix}.${key}` : key;
    return flattenKeys(nested, next);
  });
}

async function readLocale(lang, fileName) {
  const filePath = path.join(baseDir, lang, fileName);
  const raw = await readFile(filePath, 'utf8');
  return JSON.parse(raw);
}

function placeholders(value) {
  if (typeof value !== 'string') {
    return '';
  }

  const patterns = [/\{\{\s*[^}]+\s*\}\}/g];

  return patterns
    .flatMap((pattern) => [...value.matchAll(pattern)].map((match) => match[0]))
    .sort()
    .join('|');
}

function collectValueMap(value, prefix = '', out = {}) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    out[prefix] = value;
    return out;
  }

  for (const [key, nested] of Object.entries(value)) {
    const next = prefix ? `${prefix}.${key}` : key;
    collectValueMap(nested, next, out);
  }

  return out;
}

function compareLocales(reference, locale, label, issues) {
  const referenceKeys = new Set(flattenKeys(reference));
  const localeKeys = new Set(flattenKeys(locale));
  const referenceValues = collectValueMap(reference);
  const localeValues = collectValueMap(locale);

  for (const key of referenceKeys) {
    if (!localeKeys.has(key)) {
      issues.push(`${label} missing key: ${key}`);
      continue;
    }

    if (placeholders(referenceValues[key]) !== placeholders(localeValues[key])) {
      issues.push(`${label} placeholder mismatch: ${key}`);
    }
  }

  for (const key of localeKeys) {
    if (!referenceKeys.has(key)) {
      issues.push(`${label} extra key: ${key}`);
    }
  }
}

async function readJson(filePath) {
  return JSON.parse(await readFile(filePath, 'utf8'));
}

async function collectI18nDirs(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const dirs = [];

  if (entries.some((entry) => entry.isFile() && entry.name === `${referenceLang}.json`)) {
    dirs.push(dir);
  }

  for (const entry of entries) {
    if (entry.isDirectory()) {
      dirs.push(...await collectI18nDirs(path.join(dir, entry.name)));
    }
  }

  return dirs;
}

async function supportedLocaleDirs() {
  return (await readdir(baseDir, { withFileTypes: true }))
    .filter((entry) => entry.isDirectory() && entry.name !== referenceLang)
    .map((entry) => entry.name)
    .sort();
}

async function main() {
  const referenceFiles = await readdir(path.join(baseDir, referenceLang));
  const compareLangs = await supportedLocaleDirs();
  const issues = [];

  for (const fileName of referenceFiles.filter((name) => name.endsWith('.json'))) {
    const reference = await readLocale(referenceLang, fileName);

    for (const lang of compareLangs) {
      const locale = await readLocale(lang, fileName);
      compareLocales(reference, locale, `${lang}/${fileName}`, issues);
    }
  }

  for (const root of coLocatedI18nRoots) {
    const dirs = await collectI18nDirs(root);
    for (const dir of dirs) {
      const reference = await readJson(path.join(dir, `${referenceLang}.json`));

      for (const lang of compareLangs) {
        const localePath = path.join(dir, `${lang}.json`);
        try {
          const locale = await readJson(localePath);
          compareLocales(reference, locale, `${path.relative(process.cwd(), localePath)}`, issues);
        } catch {
          issues.push(`${path.relative(process.cwd(), localePath)} missing or invalid`);
        }
      }
    }
  }

  if (issues.length > 0) {
    globalThis.console.error('i18n validation failed:');
    for (const issue of issues) {
      globalThis.console.error(`- ${issue}`);
    }
    process.exit(1);
  }

  globalThis.console.log('i18n validation passed');
}

main().catch((error) => {
  globalThis.console.error(error);
  process.exit(1);
});
