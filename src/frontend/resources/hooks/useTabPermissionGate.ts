// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';

export function useTabPermissionGate<TabId extends string>(
  required: Partial<Record<TabId, string>>,
): (tab: TabId) => string | undefined {
  const { t } = useTranslation('common');
  const { data: permissions } = useCurrentUserPermissions();

  return useCallback(
    (tab: TabId) => {
      const key = required[tab];
      if (!key || hasPermission(permissions, key)) return undefined;
      return t('permission.requires', { permission: key });
    },
    [required, permissions, t],
  );
}
