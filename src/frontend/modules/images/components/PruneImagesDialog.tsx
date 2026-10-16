// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { usePruneImages } from '@resources/hooks/useImages';
import { PERM } from '@core/auth/permissionKeys';

type PruneImagesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function PruneImagesDialog({ open: isOpen, onOpenChange, onSuccess }: PruneImagesDialogProps) {
  const { t } = useTranslation('images');
  const pruneImages = usePruneImages();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('pruneUnused')}
      description={t('pruneDescription')}
      confirmLabel={t('pruneUnused')}
      permission={PERM.IMAGES_MANAGE}
      mutation={pruneImages}
      variables={undefined}
      onSuccess={onSuccess}
    />
  );
}
