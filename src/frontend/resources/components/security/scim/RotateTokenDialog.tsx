// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRotateToken, type Endpoint, type RotateTokenResponse } from '@resources/hooks/useScimEndpoints';

type RotateTokenDialogProps = {
  endpoint: Pick<Endpoint, 'id' | 'name'> | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: (result: RotateTokenResponse) => void;
};

export function RotateTokenDialog({ endpoint, onOpenChange, onSuccess }: RotateTokenDialogProps) {
  const { t } = useTranslation('scim');
  const rotate = useRotateToken();
  return (
    <MutationConfirmDialog
      open={endpoint !== null}
      onOpenChange={onOpenChange}
      title={t('rotate.title', { name: endpoint?.name ?? '' })}
      description={t('rotate.description')}
      confirmLabel={t('rotate.confirm')}
      variant="caution"
      permission={PERM.SCIM_MANAGE}
      mutation={rotate}
      variables={endpoint?.id ?? ''}
      onSuccess={onSuccess}
    />
  );
}
