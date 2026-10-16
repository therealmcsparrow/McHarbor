// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { BackupEncryptionKeyCard } from '@resources/components/settings/BackupEncryptionKeyCard';
import { BackupLogRetentionCard } from '@resources/components/settings/BackupLogRetentionCard';

/**
 * Backups → Settings: the instance-wide settings that shape every backup,
 * as opposed to the per-container schedules on the Plans tab.
 *
 * Both cards live in the settings module (their hooks do) and are composed
 * here, the same way `modules/security` composes `PasswordPolicyCard`.
 * Both are backed by `/settings/*` routes gated on `settings.view` /
 * `settings.manage`, so a user with only `backups.*` sees why the tab is
 * empty instead of a wall of failed requests.
 */
export function BackupSettingsTab() {
  const { t } = useTranslation('common');
  const canView = usePermission(PERM.SETTINGS_VIEW);

  if (!canView) {
    return (
      <Alert
        variant="warning"
        title={t('permission.denied')}
        description={t('permission.requires', { permission: PERM.SETTINGS_VIEW })}
      />
    );
  }

  return (
    <div className="space-y-4 pb-4">
      <BackupEncryptionKeyCard />
      <BackupLogRetentionCard />
    </div>
  );
}
