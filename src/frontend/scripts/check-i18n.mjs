import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const baseDir = path.resolve('core/i18n/locales');
const referenceLang = 'en';
const compareLangs = ['nl', 'de'];

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

async function main() {
  const referenceFiles = await readdir(path.join(baseDir, referenceLang));
  const issues = [];

  for (const fileName of referenceFiles.filter((name) => name.endsWith('.json'))) {
    const referenceKeys = new Set(flattenKeys(await readLocale(referenceLang, fileName)));

    for (const lang of compareLangs) {
      const localeKeys = new Set(flattenKeys(await readLocale(lang, fileName)));

      for (const key of referenceKeys) {
        if (!localeKeys.has(key)) {
          issues.push(`${lang}/${fileName} missing key: ${key}`);
        }
      }

      for (const key of localeKeys) {
        if (!referenceKeys.has(key)) {
          issues.push(`${lang}/${fileName} extra key: ${key}`);
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
