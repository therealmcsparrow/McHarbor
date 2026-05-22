// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { formatBytes, formatDate, truncateId } from '@resources/utils/format';
import type { ImageInspect } from '@core/types/docker';

type ImageOverviewCardProps = {
  image: ImageInspect;
};

export function ImageOverviewCard({ image }: ImageOverviewCardProps) {
  const { t } = useTranslation('images');

  return (
    <div className="rounded-lg border border-border bg-card p-6">
      <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        {t('detail.imageDetails')}
      </h3>
      <InfoRow label={t('detail.fields.id')}>
        <span className="font-mono text-xs">{truncateId(image.Id.replace('sha256:', ''))}</span>
      </InfoRow>
      <InfoRow label={t('detail.fields.tags')}>
        {image.RepoTags && image.RepoTags.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {image.RepoTags.map((tag) => (
              <Badge key={tag} variant="secondary">{tag}</Badge>
            ))}
          </div>
        ) : (
          '-'
        )}
      </InfoRow>
      <InfoRow label={t('detail.fields.size')}>{formatBytes(image.Size)}</InfoRow>
      <InfoRow label={t('detail.fields.created')}>{formatDate(image.Created)}</InfoRow>
      <InfoRow label={t('detail.fields.architecture')}>
        {image.Architecture}{image.Variant ? `/${image.Variant}` : ''}
      </InfoRow>
      <InfoRow label={t('detail.fields.os')}>
        {image.Os}{image.OsVersion ? ` ${image.OsVersion}` : ''}
      </InfoRow>
      <InfoRow label={t('detail.fields.dockerVersion')}>{image.DockerVersion || '-'}</InfoRow>
      <InfoRow label={t('detail.fields.author')}>{image.Author || '-'}</InfoRow>
      <InfoRow label={t('detail.fields.digests')}>
        {image.RepoDigests && image.RepoDigests.length > 0 ? (
          <div className="flex flex-col gap-1">
            {image.RepoDigests.map((digest) => (
              <span key={digest} className="break-all font-mono text-xs">{digest}</span>
            ))}
          </div>
        ) : (
          '-'
        )}
      </InfoRow>
    </div>
  );
}
