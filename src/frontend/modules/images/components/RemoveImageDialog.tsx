// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useRemoveImage } from '@resources/hooks/useImages';
import { PERM } from '@core/auth/permissionKeys';

type RemoveImageDialogProps = {
  imageId: string | null;
  blocked?: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RemoveImageDialog({ imageId, blocked, onOpenChange, onSuccess }: RemoveImageDialogProps) {
  const { t } = useTranslation('images');
  const removeImage = useRemoveImage();

  return (
    <MutationConfirmDialog
      open={imageId !== null}
      onOpenChange={onOpenChange}
      title={t('confirm.removeTitle')}
      description={t('confirm.removeDescription')}
      permission={PERM.IMAGES_DELETE}
      mutation={removeImage}
      variables={imageId ?? ''}
      blocked={blocked}
      onSuccess={onSuccess}
    />
  );
}
