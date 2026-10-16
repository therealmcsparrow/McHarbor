// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteBundle } from '@resources/hooks/useSupport';

type DeleteSupportBundleDialogProps = {
  bundleId: string | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function DeleteSupportBundleDialog({ bundleId, onOpenChange, onSuccess }: DeleteSupportBundleDialogProps) {
  const { t } = useTranslation('support');
  const remove = useDeleteBundle();
  return (
    <MutationConfirmDialog
      open={bundleId !== null}
      onOpenChange={onOpenChange}
      title={t('bundle.deleteTitle')}
      description={t('bundle.deleteDescription')}
      confirmLabel={t('bundle.delete')}
      permission={PERM.SUPPORT_MANAGE}
      mutation={remove}
      variables={bundleId ?? ''}
      onSuccess={onSuccess}
    />
  );
}
