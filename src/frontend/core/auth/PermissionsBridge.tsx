// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useAuth } from './useAuth';
import { usePermissionsBridgePublisher } from './useCurrentUserPermissions';

/**
 * Owns the host's single `/auth/me/permissions` query and republishes every
 * result on `window.McharborHost.permissions`.
 *
 * Renders nothing. It exists as a component rather than a call inside
 * `main.tsx` because the publisher needs the auth store — there is no user to
 * ask about before sign-in — and because mounting it under `AuthProvider`
 * makes the ordering explicit: the bridge is live before any plugin bundle
 * loads, so a plugin never sees an empty permission set and falls back to a
 * request of its own.
 */
export function PermissionsBridge() {
  const user = useAuth((s) => s.user);
  usePermissionsBridgePublisher(!!user);
  return null;
}
