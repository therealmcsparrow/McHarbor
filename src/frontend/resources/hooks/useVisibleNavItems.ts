// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/usePermission';
import type { NavItem } from '@resources/layout/sidebar-data';

/**
 * Filter an array of sidebar items down to the ones the current user
 * is allowed to see.
 *
 * Each item with an optional `permission` field is dropped when the
 * caller lacks it. When `permission` is a string[], ANY-of semantics
 * apply (mirrors `usePermission`). Items without a `permission` field
 * always pass through.
 *
 * The filter is intentionally a UX hide. Every backend endpoint still
 * enforces its own check on every request; this hook just keeps the
 * sidebar from advertising a link the user can't follow.
 */
export function useVisibleNavItems(items: NavItem[]): NavItem[] {
  const { data } = useCurrentUserPermissions();
  return useMemo(() => {
    return items.filter((item) => {
      if (!item.permission) return true;
      const list = Array.isArray(item.permission) ? item.permission : [item.permission];
      if (list.length === 0) return true;
      return list.some((p) => hasPermission(data, p));
    });
  }, [items, data]);
}
