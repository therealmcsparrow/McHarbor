// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';

/** PluginStorePointer tells the reader where plugin prereleases live; no plugin switch sits on the Updates page. */
export function PluginStorePointer() {
  const { t } = useTranslation('settings');
  const canOpen = usePermission(PERM.PLUGINS_VIEW);
  return (
    <p className="text-xs text-muted-foreground">
      {t('prerelease.storePointer.text')}{' '}
      {canOpen && (
        <Link to="/store?tab=plugins" className="text-primary hover:underline">
          {t('prerelease.storePointer.link')}
        </Link>
      )}
    </p>
  );
}
