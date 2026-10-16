// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess, describeApiError } from '@resources/utils/api-mutation';

export type PermissionSourceKind = 'role' | 'elevation' | 'break_glass' | 'override_grant' | 'override_deny';

export type PermissionSource = {
  kind: PermissionSourceKind;
  roleId?: string;
  roleName?: string;
  via?: 'user' | 'group';
  groupId?: string;
  groupName?: string;
  environmentId?: string;
  stackName?: string;
  wildcard?: boolean;
  elevationId?: string;
  until?: string;
  overrideId?: string;
  resourceType?: string;
  resourceId?: string;
  conditional?: boolean;
  reason?: string;
};

export type ExplainedPermission = {
  key: string;
  effective: boolean;
  authorizeGrant: boolean;
  sources: PermissionSource[];
};

export type EffectivePermissions = {
  userId: string;
  username: string;
  environmentId?: string;
  stackName?: string;
  wildcard: boolean;
  breakGlass: boolean;
  abacEnforced: boolean;
  permissions: ExplainedPermission[];
  stackScopes: string[];
  environmentNames: Record<string, string>;
};

export type EffectiveScope = {
  userId: string;
  environmentId: string;
  stack: string;
};

export function useEffectivePermissions(scope: EffectiveScope) {
  return useQuery({
    queryKey: ['access-review', 'effective', scope.userId, scope.environmentId, scope.stack],
    queryFn: () => {
      const params: Record<string, string> = { userId: scope.userId };
      if (scope.environmentId) params.environmentId = scope.environmentId;
      if (scope.stack) params.stack = scope.stack;
      return api.get<EffectivePermissions>('/access-review/effective-permissions', params).then(assertSuccess);
    },
    enabled: scope.userId !== '',
  });
}

export type CatalogExportFormat = 'csv' | 'json';

export function useExportPermissionCatalog() {
  const { t } = useTranslation('access');
  return useMutation({
    mutationFn: async (format: CatalogExportFormat) => {
      const response = await api.download('/access-review/permission-catalog/export', { format });
      if (!response.ok) {
        const body = await response.json().catch(() => ({ error: t('catalog.exportFailed') }));
        throw new Error(describeApiError(body));
      }
      const disposition = response.headers.get('Content-Disposition') ?? '';
      const match = /filename="([^"]+)"/.exec(disposition);
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = match?.[1] ?? `mcharbor-permission-catalog.${format}`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    },
    meta: { success: () => t('catalog.exported') },
  });
}
