// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { useEvictImage, useFetchImage } from '../hooks/useCompute';
import type { ComputeImage } from '../types';
import { UnverifiedImageDialog } from './UnverifiedImageDialog';
import { UploadImageDialog } from './UploadImageDialog';

interface ImageDialogsProps {
  environmentId: string;
  uploading: boolean;
  onCloseUpload: () => void;
  unverified: ComputeImage | null;
  onCloseUnverified: () => void;
  evicting: ComputeImage | null;
  onCloseEvict: () => void;
  fetchImage: ReturnType<typeof useFetchImage>;
  evict: ReturnType<typeof useEvictImage>;
}

export function ImageDialogs({
  environmentId,
  uploading,
  onCloseUpload,
  unverified,
  onCloseUnverified,
  evicting,
  onCloseEvict,
  fetchImage,
  evict,
}: ImageDialogsProps) {
  const { t } = useTranslation('compute');

  return (
    <>
      {uploading && <UploadImageDialog onClose={onCloseUpload} />}

      {unverified && (
        <UnverifiedImageDialog
          image={unverified}
          onClose={onCloseUnverified}
          onConfirm={() => {
            fetchImage.mutate({
              imageId: unverified.id,
              environmentId,
              acknowledgeUnverified: true,
            });
            onCloseUnverified();
          }}
        />
      )}

      {evicting && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && onCloseEvict()}
          title={t('images.evict.title', { name: evicting.displayName })}
          description={t('images.evict.body')}
          confirmLabel={t('images.evict.submit')}
          variant="destructive"
          onConfirm={() => {
            evict.mutate({ imageId: evicting.id, environmentId }, { onSuccess: () => onCloseEvict() });
          }}
        />
      )}
    </>
  );
}
