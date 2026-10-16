// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PERM } from '@core/auth/permissionKeys';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';

type PermissionKey = string | string[];

/**
 * Returns the key to gate an action on when it also needs host_self.manage (a McHarbor
 * container or stack). The button `permission` prop is any-of, so "own key AND
 * host_self.manage" is expressed by swapping in host_self.manage once the caller holds the
 * own key but not that one; the disabled control's tooltip then names the key missing.
 */
export function useHostSelfKey(needed: boolean): (base: PermissionKey) => PermissionKey {
  const { data } = useCurrentUserPermissions();
  const holdsHostSelf = hasPermission(data, PERM.HOST_SELF_MANAGE);
  return (base) => {
    if (!needed || holdsHostSelf) return base;
    const keys = Array.isArray(base) ? base : [base];
    return keys.some((key) => hasPermission(data, key)) ? PERM.HOST_SELF_MANAGE : base;
  };
}
