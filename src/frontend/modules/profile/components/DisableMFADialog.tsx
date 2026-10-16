// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useMFADisable } from '@resources/hooks/useMFA';

type DisableMFADialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function DisableMFADialog({ open: isOpen, onOpenChange, onSuccess }: DisableMFADialogProps) {
  const { t } = useTranslation('common');
  const disable = useMFADisable();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('mfa.disableTitle')}
      description={t('mfa.confirmDisable')}
      confirmLabel={t('mfa.disable')}
      permission={PERM.ACCOUNT_SELF_MANAGE}
      mutation={disable}
      variables={undefined}
      onSuccess={onSuccess}
    />
  );
}
