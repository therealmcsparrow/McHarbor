// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRemoveNetwork } from '../hooks/useNetworks';

type RemoveNetworkDialogProps = {
  networkId: string | null;
  builtIn?: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RemoveNetworkDialog({ networkId, builtIn = false, onOpenChange, onSuccess }: RemoveNetworkDialogProps) {
  const { t } = useTranslation('networks');
  const removeNetwork = useRemoveNetwork();

  return (
    <MutationConfirmDialog
      open={networkId !== null}
      onOpenChange={onOpenChange}
      title={builtIn ? t('confirm.builtInRemoveTitle') : t('confirm.removeTitle')}
      description={builtIn ? t('confirm.builtInRemoveDescription') : t('confirm.removeDescription')}
      permission={PERM.NETWORKS_DELETE}
      mutation={removeNetwork}
      variables={networkId ?? ''}
      onSuccess={onSuccess}
    />
  );
}
