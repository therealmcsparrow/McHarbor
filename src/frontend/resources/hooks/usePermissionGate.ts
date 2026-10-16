// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/usePermission';

/**
 * The shape every gated affordance needs: may I act, and if not, what do I
 * tell the user?
 *
 * `allowed` is true when no permission is required, so adding the hook to a
 * component does not change the behaviour of call sites that pass nothing.
 */
export type PermissionGate = {
  allowed: boolean;
  /** True only when a permission was required and the caller lacks it. */
  denied: boolean;
  /** Translated explanation, or '' when allowed. */
  reason: string;
};

/**
 * usePermissionGate resolves one permission (or an any-of list) into the
 * three things a button needs to render itself honestly.
 *
 * This is the UX half of RBAC. Every backend endpoint still enforces its own
 * check on every request — a caller who defeats the disabled state in devtools
 * gets a 403, not an action. What this buys is that a view-only user can see
 * which controls exist and which permission would unlock them, instead of
 * discovering it from an error toast after the fact.
 *
 * Denied controls are disabled rather than hidden so a table's action column
 * keeps its width across roles; a viewer and an admin looking at the same list
 * see the same layout.
 */
export function usePermissionGate(permission?: string | string[]): PermissionGate {
  const { t } = useTranslation('common');
  const { data } = useCurrentUserPermissions();

  const list = permission === undefined ? [] : Array.isArray(permission) ? permission : [permission];

  if (list.length === 0) {
    return { allowed: true, denied: false, reason: '' };
  }

  const allowed = list.some((p) => hasPermission(data, p));
  if (allowed) {
    return { allowed: true, denied: false, reason: '' };
  }

  // An any-of list names the first key, which is the one a caller would
  // normally be granted; spelling out every alternative reads as noise in a
  // tooltip that has one line to work with.
  return {
    allowed: false,
    denied: true,
    reason: t('permission.requires', { permission: list[0] }),
  };
}
