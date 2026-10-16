// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useRestartContainer } from '../hooks/useOnCallContainer';

type RestartDialogProps = {
  envId: string;
  containerId: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RestartDialog({
  envId,
  containerId,
  name: itemName,
  open: isOpen,
  onOpenChange,
  onSuccess,
}: RestartDialogProps) {
  const { t } = useTranslation('oncall');
  const restart = useRestartContainer();
  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('container.restartTitle', { name: itemName })}
      description={t('container.restartDescription')}
      confirmLabel={t('container.restart')}
      variant="caution"
      permission={PERM.CONTAINERS_MANAGE}
      mutation={restart}
      variables={{ envId, containerId }}
      onSuccess={onSuccess}
    />
  );
}
