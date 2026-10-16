// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ImageInfo } from '@core/types/docker';
import { ResourcePicker, type ResourcePickerItem } from './ResourcePicker';

interface ImageSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
  allowCustom?: boolean;
}

function formatSize(bytes: number): string {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

/** Picks a local image by tag (falls back to the short id for untagged images). */
export function ImageSelect({ value, onChange, envId, allowCustom }: ImageSelectProps) {
  const { t } = useTranslation('common');

  const { data: images, isLoading } = useQuery({
    queryKey: ['images-select', envId ?? ''],
    queryFn: () => api.get<ImageInfo[]>('/images', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  const items: ResourcePickerItem[] = [];
  for (const image of images ?? []) {
    const shortId = image.Id.replace(/^sha256:/, '').slice(0, 12);
    const tags = (image.RepoTags ?? []).filter((tag) => tag !== '<none>:<none>');
    if (tags.length === 0) {
      items.push({ value: shortId, title: shortId, subtitle: formatSize(image.Size) });
      continue;
    }
    for (const tag of tags) {
      items.push({
        value: tag,
        title: tag,
        subtitle: `${shortId} - ${formatSize(image.Size)}`,
      });
    }
  }

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      allowCustom={allowCustom}
      placeholder={t('workflows.selectImage')}
      emptyLabel={t('workflows.noImagesFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={items}
    />
  );
}
