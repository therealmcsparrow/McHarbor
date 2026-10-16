// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteEndpoint, type Endpoint } from '@resources/hooks/useScimEndpoints';

type DeleteEndpointDialogProps = {
  endpoint: Pick<Endpoint, 'id' | 'name'> | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function DeleteEndpointDialog({ endpoint, onOpenChange, onSuccess }: DeleteEndpointDialogProps) {
  const { t } = useTranslation('scim');
  const remove = useDeleteEndpoint();
  return (
    <MutationConfirmDialog
      open={endpoint !== null}
      onOpenChange={onOpenChange}
      title={t('delete.title')}
      description={t('delete.description', { name: endpoint?.name ?? '' })}
      confirmLabel={t('delete.confirm')}
      permission={PERM.SCIM_MANAGE}
      mutation={remove}
      variables={endpoint?.id ?? ''}
      onSuccess={onSuccess}
    >
      <p className="text-sm text-muted-foreground">{t('delete.warning')}</p>
    </MutationConfirmDialog>
  );
}
