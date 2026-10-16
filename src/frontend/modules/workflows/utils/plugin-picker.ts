// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PluginPickerList } from '@core/plugins/loader';
import type { ResourcePickerItem } from '../components/ResourcePicker';
import { displayValue } from './display-value';

/**
 * The list request's query. Mirrors callQuery in modules/runbooks: `$env` is the
 * node's environment and is left out without one, so the plugin lists every
 * environment the operator may see; `cluster` has no meaning outside a runbook.
 */
export function pickerQuery(picker: PluginPickerList, envId?: string): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const [key, source] of Object.entries(picker.query ?? {})) {
    if (source === '$env') {
      if (envId) out[key] = envId;
    } else if (source.startsWith('=')) {
      out[key] = source.slice(1);
    }
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

function field(item: Record<string, unknown>, key: string | undefined): string {
  if (!key) return '';
  const value = item[key];
  return value === undefined || value === null ? '' : displayValue(value).trim();
}

/** Mirrors parseTargets in modules/runbooks, so a picker and a runbook selector agree on ids. */
export function pickerItems(picker: PluginPickerList, data: unknown): ResourcePickerItem[] {
  let rows: unknown = data;
  if (picker.items) {
    rows = data && typeof data === 'object' ? (data as Record<string, unknown>)[picker.items] : undefined;
  }
  if (!Array.isArray(rows)) return [];
  const out: ResourcePickerItem[] = [];
  for (const row of rows) {
    if (!row || typeof row !== 'object') continue;
    const item = row as Record<string, unknown>;
    const id = picker.idJoin?.length ? picker.idJoin.map((k) => field(item, k)).join('/') : field(item, picker.id);
    if (id.replace(/\//g, '') === '') continue;
    const title = picker.name.map((k) => field(item, k)).find((v) => v !== '') ?? id;
    const detail = (picker.detail ?? [])
      .map((k) => field(item, k))
      .filter((v) => v !== '')
      .join(' · ');
    out.push({ value: id, title, subtitle: detail || undefined });
  }
  return out;
}
