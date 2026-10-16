// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBellRinging, IconKey } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { PERM } from '@core/auth/permissionKeys';
import { RegenerateVapidDialog } from './RegenerateVapidDialog';

/**
 * Settings → Notifications → Web push. The VAPID key pair is one per
 * instance, so regenerating it is the only administrative push action and
 * it is destructive: it detaches every device at once.
 */
export function PushDeliveryCard() {
  const { t } = useTranslation('settings');
  const [confirmOpen, setConfirmOpen] = useState(false);

  return (
    <SettingsGroup
      icon={IconBellRinging}
      title={t('notifications.pushTitle')}
      description={t('notifications.pushDescription')}
      scope={t('general.scopeGlobal')}
    >
      <div className="flex flex-wrap items-center gap-3">
        <Button
          permission={PERM.SETTINGS_MANAGE}
          type="button"
          variant="destructive"
          size="sm"
          onClick={() => setConfirmOpen(true)}
        >
          <IconKey className="size-4" aria-hidden="true" />
          {t('notifications.pushRegenerate')}
        </Button>
        <span className="text-xs text-muted-foreground">{t('notifications.pushRegenerateHint')}</span>
      </div>

      <RegenerateVapidDialog open={confirmOpen} onOpenChange={setConfirmOpen} />
    </SettingsGroup>
  );
}
