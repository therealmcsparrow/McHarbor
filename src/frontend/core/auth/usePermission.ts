// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { PERM } from './permissionKeys';
import { hasPermission, useCurrentUserPermissions } from './useCurrentUserPermissions';

export { hasPermission, useCurrentUserPermissions };

export type PermissionCatalogEntry = {
  key: string;
  category: string;
  action: string;
  description: string;
  scope: 'global' | 'environment' | 'stack' | 'owner' | 'account';
  /**
   * Whether this key may be requested as a temporary grant through
   * Security -> Elevations. The flag lives in the permission_catalog table
   * rather than the code-side seed, so the backend reads it back from the
   * database; entries from before that field existed simply arrive undefined.
   */
  elevatable?: boolean;
};

export const PERMISSION_CATALOG_STALE_TIME = 5 * 60_000;

/**
 * usePermissionCatalog fetches the canonical list of permissions.
 * Used by the role editor and permission-picker UI so the labels
 * come from the backend (single source of truth).
 */
export function usePermissionCatalog() {
  // The route accepts either key — roles.view keeps existing role authors
  // working, permissions.view is the dedicated one. Asking without either
  // returns a 403 the picker cannot render.
  const allowed = usePermission([PERM.ROLES_VIEW, PERM.PERMISSIONS_VIEW]);
  return useQuery({
    enabled: allowed,
    queryKey: ['auth', 'permissions', 'catalog'],
    queryFn: () => api.get<PermissionCatalogEntry[]>('/auth/permissions/catalog').then((r) => r.data ?? []),
    staleTime: PERMISSION_CATALOG_STALE_TIME,
  });
}

/**
 * usePermission returns true when the current user holds the given
 * permission. Reads from the cached /auth/me/permissions response so
 * callers do not need to coordinate queries.
 *
 * Pass an array of permissions to check "any of"; the first match
 * wins. An empty array always returns false.
 */
export function usePermission(permission: string | string[]): boolean {
  const { data } = useCurrentUserPermissions();
  const list = Array.isArray(permission) ? permission : [permission];
  if (list.length === 0) return false;
  return list.some((p) => hasPermission(data, p));
}

/**
 * useAnyPermission returns true if the caller has AT LEAST ONE of the
 * listed permissions. Convenient for menus that should appear if any
 * matching gate is open.
 */
export function useAnyPermission(permissions: string[]): boolean {
  return usePermission(permissions);
}

/**
 * useAllPermissions returns true only when the caller has every listed
 * permission. Use sparingly — most "all" checks are accidental ANDs that
 * should be ORs.
 */
export function useAllPermissions(permissions: string[]): boolean {
  const { data } = useCurrentUserPermissions();
  if (permissions.length === 0) return true;
  return permissions.every((p) => hasPermission(data, p));
}
