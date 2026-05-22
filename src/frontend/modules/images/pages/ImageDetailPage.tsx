// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@resources/components/ui/Spinner';
import { useImage, useImageHistory, useRemoveImage, useExportImage } from '../hooks/useImages';
import { ImageConfigCard } from '../components/ImageConfigCard';
import { ImageDetailHeader } from '../components/ImageDetailHeader';
import { ImageHistoryCard } from '../components/ImageHistoryCard';
import { ImageLabelsCard } from '../components/ImageLabelsCard';
import { ImageOverviewCard } from '../components/ImageOverviewCard';

export default function ImageDetailPage() {
  const { t } = useTranslation('images');
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: image, isLoading } = useImage(id ?? '');
  const { data: history = [] } = useImageHistory(id ?? '');
  const removeImage = useRemoveImage();
  const { exportImage } = useExportImage();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!image) {
    return (
      <div className="py-12 text-center text-muted-foreground">{t('notFound')}</div>
    );
  }

  const handleRemove = () => {
    removeImage.mutate(image.Id, {
      onSuccess: () => navigate('/images'),
    });
  };

  return (
    <div className="space-y-6">
      <ImageDetailHeader
        image={image}
        onBack={() => navigate('/images')}
        onExport={() => exportImage(image.Id, image.RepoTags?.[0] ?? image.Id.replace('sha256:', '').slice(0, 12))}
        onRemove={handleRemove}
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <ImageOverviewCard image={image} />
        <ImageConfigCard image={image} />
        <ImageLabelsCard image={image} />
        <ImageHistoryCard history={history} />
      </div>
    </div>
  );
}
