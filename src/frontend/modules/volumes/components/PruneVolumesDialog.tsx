// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { usePruneVolumes } from '../hooks/useVolumes';

type PruneVolumesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function PruneVolumesDialog({ open: isOpen, onOpenChange, onSuccess }: PruneVolumesDialogProps) {
  const { t } = useTranslation('volumes');
  const pruneVolumes = usePruneVolumes();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('pruneUnused')}
      description={t('pruneDescription')}
      confirmLabel={t('pruneUnused')}
      permission={PERM.VOLUMES_MANAGE}
      mutation={pruneVolumes}
      variables={undefined}
      onSuccess={onSuccess}
    />
  );
}
