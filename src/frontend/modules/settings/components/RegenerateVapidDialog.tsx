// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRegenerateVapidKey } from '../hooks/usePushVapid';

type RegenerateVapidDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RegenerateVapidDialog({ open: isOpen, onOpenChange, onSuccess }: RegenerateVapidDialogProps) {
  const { t } = useTranslation('settings');
  const regenerate = useRegenerateVapidKey();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('notifications.pushConfirmTitle')}
      description={t('notifications.pushConfirmDescription')}
      confirmLabel={t('notifications.pushConfirmAction')}
      variant="destructive"
      permission={PERM.SETTINGS_MANAGE}
      mutation={regenerate}
      variables={undefined}
      onSuccess={() => onSuccess?.()}
    />
  );
}
