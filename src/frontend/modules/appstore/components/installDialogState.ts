// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AppNetworkSettings, AppTemplate } from '../types';

export type EnvEntry = {
  key: string;
  value: string;
};

export function createDefaultNetworkSettings(): AppNetworkSettings {
  return {
    mode: 'default',
    name: '',
    aliases: [],
    ipv4Address: '',
    ipv6Address: '',
    macAddress: '',
  };
}

export function entriesToEnvVars(entries: EnvEntry[]): Record<string, string> {
  return entries.reduce<Record<string, string>>((acc, entry) => {
    const key = entry.key.trim();
    if (key) {
      acc[key] = entry.value;
    }
    return acc;
  }, {});
}

export function createDefaultEnvEntries(app: AppTemplate, timezone?: string): EnvEntry[] {
  const defaults = new Map<string, string>();
  app.envVars.forEach((ev) => {
    defaults.set(ev.key, ev.default);
  });
  defaults.set('TZ', timezone?.trim() || 'UTC');
  return Array.from(defaults, ([key, value]) => ({ key, value }));
}
