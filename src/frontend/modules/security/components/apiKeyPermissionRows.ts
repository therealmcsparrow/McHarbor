// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Row model for the API-key permission matrix.
//
// The API returns four sets — role grants, direct grants, their union, and
// that union narrowed by the owner's own rights. Drawing them as four walls
// of raw keys leaves the reader to diff two near-identical sets by eye, so
// they fold into a single list keyed on the *category*: 149 permissions
// spread over 64 categories, and a read-only role holds exactly one
// permission in each of 37 of them, so a row per category with its granted
// actions beside it answers "what can this key touch, and how deeply" in as
// many lines as there are subjects.

import type { PermissionCatalogEntry } from '@core/auth/usePermission';
import type { APIKeyDetail } from '../hooks/useAPIKeys';

/** Where a permission came from. `direct` means granted on the key itself. */
export type PermissionSource = { id: string; label: string; direct: boolean };

export type ActionCell = {
  key: string;
  action: string;
  description: string;
  sources: PermissionSource[];
  effective: boolean;
};

export type CategoryRow = {
  category: string;
  label: string;
  actions: ActionCell[];
  blocked: number;
};

/** `swarm_configs` -> `Swarm Configs`, for a category the catalog has no label for. */
export function prettify(slug: string): string {
  return slug
    .split(/[._]/)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * Fold the API's four permission sets into one row per category.
 *
 * The wildcard is deliberately not expanded into rows: enumerating the whole
 * catalog would bury the handful of grants an operator actually configured.
 * It is reported as a banner instead.
 */
export function buildRows(
  detail: APIKeyDetail,
  catalog: PermissionCatalogEntry[],
  labelFor: (category: string) => string,
): CategoryRow[] {
  const byKey = new Map(catalog.map((entry) => [entry.key, entry]));
  const direct = new Set(detail.scopes);
  const effective = new Set(detail.effectivePermissions);
  const ownerHoldsEverything = effective.has('*');

  const byCategory = new Map<string, ActionCell[]>();
  for (const key of detail.configuredPermissions) {
    if (key === '*') continue;
    const entry = byKey.get(key);
    // A key not in the catalog still has to render — role presets can carry
    // permissions the catalog never declared, and hiding them would make the
    // key look narrower than it is.
    const category = entry?.category ?? key.split('.')[0] ?? key;
    const action = entry?.action ?? key.slice(category.length + 1) ?? key;

    const sources: PermissionSource[] = detail.roleDetails
      .filter((role) => role.permissions.includes(key) || role.permissions.includes('*'))
      .map((role) => ({ id: role.id, label: role.name, direct: false }));
    if (direct.has(key)) {
      sources.push({ id: `direct:${key}`, label: key, direct: true });
    }

    const cell: ActionCell = {
      key,
      action: action || key,
      description: entry?.description ?? '',
      sources,
      effective: ownerHoldsEverything || effective.has(key),
    };
    const bucket = byCategory.get(category);
    if (bucket) bucket.push(cell);
    else byCategory.set(category, [cell]);
  }

  return [...byCategory.entries()]
    .map(([category, actions]) => {
      actions.sort((a, b) => a.action.localeCompare(b.action));
      return {
        category,
        label: labelFor(category),
        actions,
        blocked: actions.filter((cell) => !cell.effective).length,
      };
    })
    .sort((a, b) => a.label.localeCompare(b.label));
}

/** Rows narrowed by the search box and the "blocked only" toggle. */
export function filterRows(rows: CategoryRow[], filter: string, blockedOnly: boolean): CategoryRow[] {
  const needle = filter.trim().toLowerCase();
  return rows
    .map((row) => {
      const actions = row.actions.filter((cell) => {
        if (blockedOnly && cell.effective) return false;
        if (!needle) return true;
        return (
          cell.key.toLowerCase().includes(needle) ||
          cell.action.toLowerCase().includes(needle) ||
          cell.description.toLowerCase().includes(needle) ||
          row.label.toLowerCase().includes(needle) ||
          cell.sources.some((source) => source.label.toLowerCase().includes(needle))
        );
      });
      return { ...row, actions };
    })
    .filter((row) => row.actions.length > 0);
}
