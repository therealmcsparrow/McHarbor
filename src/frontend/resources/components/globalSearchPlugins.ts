// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';
import {
  IconApps,
  IconBox,
  IconBoxMultiple,
  IconCloudNetwork,
  IconCpu,
  IconDatabase,
  IconDeviceDesktop,
  IconDownload,
  IconFileUpload,
  IconKey,
  IconNetwork,
  IconPhoto,
  IconPlus,
  IconRocket,
  IconRoute,
  IconServer,
  IconServer2,
  IconServerBolt,
  IconSettings,
  IconStack2,
  IconTemplate,
  IconTopologyStar3,
} from '@tabler/icons-react';
import { fillPath } from '@core/plugins/capabilities';
import type { PluginSearchFanOut, PluginSearchSource } from '@core/plugins/pickerTypes';

export type SearchIconComponent = ComponentType<{ className?: string }>;

/** Must stay in sync with src/contracts/plugincaps/search_icons.yaml; a test holds it there. */
export const SEARCH_ICONS: Record<string, SearchIconComponent> = {
  apps: IconApps,
  box: IconBox,
  'box-multiple': IconBoxMultiple,
  'cloud-network': IconCloudNetwork,
  cpu: IconCpu,
  database: IconDatabase,
  'device-desktop': IconDeviceDesktop,
  download: IconDownload,
  'file-upload': IconFileUpload,
  key: IconKey,
  network: IconNetwork,
  photo: IconPhoto,
  plus: IconPlus,
  rocket: IconRocket,
  route: IconRoute,
  server: IconServer,
  'server-2': IconServer2,
  'server-bolt': IconServerBolt,
  settings: IconSettings,
  'stack-2': IconStack2,
  template: IconTemplate,
  'topology-star-3': IconTopologyStar3,
};

export function searchIcon(iconName: string): SearchIconComponent {
  return SEARCH_ICONS[iconName] ?? IconApps;
}

export type SearchEnv = { id: string; name: string };

/** One item of a source's fan-out list: the value sent as its parameter, and how it reads. */
export type FanOutTarget = { param: string; id: string; label: string };

/** Mirrors plugincaps.MaxFanOutTargets: the palette never sends more requests per source. */
export const MAX_FAN_OUT_TARGETS = 16;

export type PluginSearchItem = {
  key: string;
  label: string;
  detail: string;
  to: string;
  envId?: string;
  envName?: string;
};

function field(item: Record<string, unknown>, key: string): string {
  const value = item[key];
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
    ? String(value).trim()
    : '';
}

function at(data: unknown, path: string): unknown {
  let current = data;
  for (const part of path.split('.')) {
    if (!current || typeof current !== 'object') return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

/** The targets a fan-out list names, capped at MAX_FAN_OUT_TARGETS; rows without an id are skipped. */
export function fanOutTargets(fanOut: PluginSearchFanOut, data: unknown): FanOutTarget[] {
  const rows = fanOut.items ? at(data, fanOut.items) : data;
  if (!Array.isArray(rows)) return [];
  const out: FanOutTarget[] = [];
  for (const row of rows) {
    if (out.length >= MAX_FAN_OUT_TARGETS) break;
    if (!row || typeof row !== 'object') continue;
    const item = row as Record<string, unknown>;
    const id = field(item, fanOut.id);
    if (!id) continue;
    out.push({ param: fanOut.param, id, label: (fanOut.label && field(item, fanOut.label)) || id });
  }
  return out;
}

/**
 * Maps one response of a declared source to palette items. An item whose link still has
 * an unfilled placeholder is dropped: it would navigate to a literal `{id}`. A fan-out
 * target fills its parameter's placeholder and names itself in the detail line.
 */
export function pluginSearchItems(
  source: PluginSearchSource,
  data: unknown,
  env?: SearchEnv,
  target?: FanOutTarget,
): PluginSearchItem[] {
  const rows = source.items ? at(data, source.items) : data;
  if (!Array.isArray(rows)) return [];
  const placeholders = [...source.link.matchAll(/\{([^}]+)\}/g)].map((m) => m[1] ?? '');
  const out: PluginSearchItem[] = [];
  for (const row of rows) {
    if (!row || typeof row !== 'object') continue;
    const item = row as Record<string, unknown>;
    const id = field(item, source.id);
    if (!id) continue;
    const values: Record<string, string> = {};
    for (const placeholder of placeholders) {
      const value = field(item, placeholder) || (target && placeholder === target.param ? target.id : '');
      if (value) values[placeholder] = value;
    }
    const to = fillPath(source.link, values);
    if (/\{[^}]*\}/.test(to)) continue;
    const label =
      source.name
        .map((k) => field(item, k))
        .filter(Boolean)
        .join(' ') || id;
    const parts = (source.detail ?? []).map((k) => field(item, k)).filter(Boolean);
    if (target) parts.push(target.label);
    const detail = parts.join(' · ');
    out.push({
      key: `${env?.id ?? ''}|${target?.id ?? ''}|${to}|${id}`,
      label,
      detail,
      to,
      envId: env?.id,
      envName: env?.name,
    });
  }
  return out;
}
