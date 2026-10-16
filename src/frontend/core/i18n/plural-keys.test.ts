// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readdirSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { i18n } from '../../test/i18n';

const LOCALES_DIR = resolve(__dirname, 'locales');
const LANGS = ['en', 'nl', 'de', 'es', 'fr', 'pt'];

// Labels with no count to pluralise against, and one sentence carrying two independent counts.
const ACCEPTED_PSEUDO_PLURALS = new Set([
  'common:schedule.units.hour',
  'common:schedule.units.day',
  'common:schedule.units.week',
  'common:schedule.units.month',
  'common:schedule.units.year',
  'containers:backups.retentionCountDays',
]);

function flatten(value: unknown, prefix = '', out: Record<string, string> = {}): Record<string, string> {
  if (typeof value === 'string') out[prefix] = value;
  else if (value && typeof value === 'object') {
    for (const [key, child] of Object.entries(value)) flatten(child, prefix ? `${prefix}.${key}` : key, out);
  }
  return out;
}

function bundle(lang: string): Record<string, string> {
  const flat: Record<string, string> = {};
  for (const file of readdirSync(resolve(LOCALES_DIR, lang))) {
    if (!file.endsWith('.json')) continue;
    const ns = file.replace(/\.json$/, '');
    const parsed: unknown = JSON.parse(readFileSync(resolve(LOCALES_DIR, lang, file), 'utf8'));
    for (const [key, text] of Object.entries(flatten(parsed))) flat[`${ns}:${key}`] = text;
  }
  return flat;
}

describe('plural keys', () => {
  it('leaves no (s) pseudo-plural in the English bundles outside the accepted set', () => {
    const offenders = Object.entries(bundle('en'))
      .filter(([key, text]) => text.includes('(s)') && !ACCEPTED_PSEUDO_PLURALS.has(key))
      .map(([key]) => key);
    expect(offenders, 'use <key>_one / <key>_other and pass count').toEqual([]);
  });

  it.each(LANGS)('%s gives every _one key an _other (or bare) fallback', (lang) => {
    const keys = Object.keys(bundle(lang));
    const orphans = keys.filter(
      (key) =>
        key.endsWith('_one') &&
        !keys.includes(key.replace(/_one$/, '_other')) &&
        !keys.includes(key.replace(/_one$/, '')),
    );
    expect(orphans).toEqual([]);
  });

  it('picks the singular and plural form from count', () => {
    expect(i18n.t('access:impact.legend', { count: 1, gained: 1, lost: 0 })).toBe('1 role gained, 0 lost');
    expect(i18n.t('access:impact.legend', { count: 2, gained: 2, lost: 1 })).toBe('2 roles gained, 1 lost');
    expect(i18n.t('oncall:device.testResult', { count: 1, sent: 1, failed: 0, removed: 0 })).toBe(
      'Sent to 1 device, 0 failed, 0 removed.',
    );
    expect(i18n.t('secrets:impact.staleTitle', { count: 1 })).toBe('1 resource is out of date');
    expect(i18n.t('secrets:impact.staleTitle', { count: 4 })).toBe('4 resources are out of date');
  });
});
