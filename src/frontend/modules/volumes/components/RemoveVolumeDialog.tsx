// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useRemoveVolume } from '../hooks/useVolumes';

type RemoveVolumeDialogProps = {
  volumeName: string | null;
  blocked?: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RemoveVolumeDialog({ volumeName, blocked, onOpenChange, onSuccess }: RemoveVolumeDialogProps) {
  const { t } = useTranslation('volumes');
  const removeVolume = useRemoveVolume();

  return (
    <MutationConfirmDialog
      open={volumeName !== null}
      onOpenChange={onOpenChange}
      title={t('confirm.removeTitle')}
      description={t('confirm.removeDescription')}
      permission={PERM.VOLUMES_DELETE}
      mutation={removeVolume}
      variables={volumeName ?? ''}
      blocked={blocked}
      onSuccess={onSuccess}
    />
  );
}
