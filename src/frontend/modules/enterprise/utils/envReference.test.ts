// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import envref from '../../../core/i18n/locales/en/envref.json';
import { ENV_SECTION_ORDER, envVariables, filterEnvVariables, groupBySection, type EnvVariable } from './envReference';

const v = (name: string, extra: Partial<EnvVariable> = {}): EnvVariable => ({
  name,
  scope: 'server',
  section: 'application',
  defaultValue: '',
  description: '',
  sensitive: false,
  restartRequired: true,
  documented: true,
  inCode: true,
  sources: [],
  ...extra,
});

describe('environment variable reference', () => {
  const vars = [
    v('PORT', { description: 'Port for the main listener' }),
    v('ENCRYPTION_KEY', { section: 'secrets', sensitive: true }),
    v('MCHARBOR_URL', { scope: 'agent', section: 'agent' }),
    v('ODD', { section: 'nonsense' }),
  ];

  it('filters by name, description, scope and sensitivity together', () => {
    expect(
      filterEnvVariables(vars, { query: 'listener', scope: 'all', sensitiveOnly: false }).map((x) => x.name),
    ).toEqual(['PORT']);
    expect(
      filterEnvVariables(vars, { query: 'mcharbor_url', scope: 'all', sensitiveOnly: false }).map((x) => x.name),
    ).toEqual(['MCHARBOR_URL']);
    expect(filterEnvVariables(vars, { query: '', scope: 'agent', sensitiveOnly: false }).map((x) => x.name)).toEqual([
      'MCHARBOR_URL',
    ]);
    expect(filterEnvVariables(vars, { query: '', scope: 'all', sensitiveOnly: true }).map((x) => x.name)).toEqual([
      'ENCRYPTION_KEY',
    ]);
    expect(filterEnvVariables(vars, { query: 'port', scope: 'agent', sensitiveOnly: false })).toEqual([]);
  });

  it('groups in reference order and folds unknown sections into other', () => {
    expect(groupBySection(vars).map((g) => g.section)).toEqual(['application', 'secrets', 'agent', 'other']);
  });

  it('ships generated data that covers the secrets, flags them, and has a label for every section', () => {
    const data = envVariables();
    const byName = new Map(data.filter((x) => x.scope === 'server').map((x) => [x.name, x]));
    for (const name of ['ENCRYPTION_KEY', 'MCHARBOR_SECRET', 'MCHARBOR_DB_DSN', 'MCHARBOR_BACKUP_KEY']) {
      expect(byName.get(name)?.sensitive, name).toBe(true);
    }
    for (const name of ['PORT', 'DATA_DIR', 'BACKUP_ENCRYPTION_KEY_FILE', 'MCHARBOR_TRUSTED_PROXY_CIDRS']) {
      expect(byName.get(name)?.sensitive, name).toBe(false);
    }
    const keys = new Set(data.map((x) => `${x.scope}:${x.name}`));
    expect(keys.size).toBe(data.length);
    for (const section of ENV_SECTION_ORDER) {
      expect(envref.sections).toHaveProperty(section);
    }
    for (const x of data) {
      expect(ENV_SECTION_ORDER as readonly string[]).toContain(x.section);
    }
  });
});
