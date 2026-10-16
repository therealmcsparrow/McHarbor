// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import reference from '../data/env-reference.json';

export type EnvScope = 'server' | 'agent';

export type EnvVariable = {
  name: string;
  scope: EnvScope;
  section: string;
  defaultValue: string;
  description: string;
  sensitive: boolean;
  restartRequired: boolean;
  documented: boolean;
  inCode: boolean;
  sources: string[];
};

export type EnvFilter = {
  query: string;
  scope: EnvScope | 'all';
  sensitiveOnly: boolean;
};

export const ENV_SECTION_ORDER = [
  'application',
  'secrets',
  'database',
  'docker',
  'tls',
  'networkTrust',
  'agentTransport',
  'plugins',
  'nativeInstall',
  'authentication',
  'versionReporting',
  'agent',
  'other',
] as const;

export function envVariables(): EnvVariable[] {
  return (reference.variables as EnvVariable[]).map((v) => ({ ...v, scope: v.scope === 'agent' ? 'agent' : 'server' }));
}

export function filterEnvVariables(vars: EnvVariable[], filter: EnvFilter): EnvVariable[] {
  const q = filter.query.trim().toLowerCase();
  return vars.filter((v) => {
    if (filter.scope !== 'all' && v.scope !== filter.scope) return false;
    if (filter.sensitiveOnly && !v.sensitive) return false;
    if (!q) return true;
    return v.name.toLowerCase().includes(q) || v.description.toLowerCase().includes(q);
  });
}

export function groupBySection(vars: EnvVariable[]): { section: string; items: EnvVariable[] }[] {
  const known = new Set<string>(ENV_SECTION_ORDER);
  const groups = new Map<string, EnvVariable[]>();
  for (const v of vars) {
    const section = known.has(v.section) ? v.section : 'other';
    groups.set(section, [...(groups.get(section) ?? []), v]);
  }
  return ENV_SECTION_ORDER.filter((s) => groups.has(s)).map((section) => ({
    section,
    items: (groups.get(section) ?? []).slice().sort((a, b) => a.name.localeCompare(b.name)),
  }));
}
