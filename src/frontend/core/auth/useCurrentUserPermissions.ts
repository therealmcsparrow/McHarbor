// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type CurrentUserPermissions = {
  permissions: string[];
  wildcard: boolean;
};

export type SystemRestartResponse = {
  message: string;
  code: string;
  result: {
    containerId: string;
    containerName: string;
    scheduledAt: string;
  };
};

export const SYSTEM_RESTART_PERMISSION = 'system.manage';

export function useCurrentUserPermissions(enabled = true) {
  return useQuery({
    queryKey: ['auth', 'me', 'permissions'],
    queryFn: () =>
      api
        .get<CurrentUserPermissions>('/auth/me/permissions')
        .then((r) => r.data ?? { permissions: [], wildcard: false }),
    enabled,
    staleTime: 60_000,
  });
}

export function hasPermission(
  permissions: CurrentUserPermissions | undefined,
  permission: string,
): boolean {
  if (!permissions) {
    return false;
  }
  if (permissions.wildcard) {
    return true;
  }
  return permissions.permissions.includes(permission);
}