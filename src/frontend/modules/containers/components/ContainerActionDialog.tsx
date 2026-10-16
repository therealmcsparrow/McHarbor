// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useContainerAction } from '../hooks/useContainers';

type ContainerActionDialogProps = {
  containerId: string;
  action: 'stop' | 'kill';
  open: boolean;
  blocked?: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function ContainerActionDialog({
  containerId,
  action,
  open: isOpen,
  blocked,
  onOpenChange,
  onSuccess,
}: ContainerActionDialogProps) {
  const { t } = useTranslation('containers');
  const containerAction = useContainerAction();
  const isStop = action === 'stop';

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={isStop ? t('confirm.stopTitle') : t('confirm.killTitle')}
      description={isStop ? t('confirm.stopDescription') : t('confirm.killDescription')}
      confirmLabel={isStop ? t('actions.stop') : t('actions.kill')}
      permission={PERM.CONTAINERS_MANAGE}
      mutation={containerAction}
      variables={{ id: containerId, action }}
      blocked={blocked}
      onSuccess={onSuccess}
    />
  );
}
