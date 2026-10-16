// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { VolumeInfo } from '@core/types/docker';
import { ResourcePicker } from './ResourcePicker';

interface VolumeSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
}

/** Picks a Docker volume by name from the selected environment. */
export function VolumeSelect({ value, onChange, envId }: VolumeSelectProps) {
  const { t } = useTranslation('common');

  const { data: volumes, isLoading } = useQuery({
    queryKey: ['volumes-select', envId ?? ''],
    queryFn: () => api.get<VolumeInfo[]>('/volumes', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      placeholder={t('workflows.selectVolume')}
      emptyLabel={t('workflows.noVolumesFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={(volumes ?? []).map((volume) => ({
        value: volume.Name,
        title: volume.Name,
        subtitle: `${volume.Driver} - ${volume.Mountpoint}`,
        badge: volume.Protected ? t('workflows.protectedBadge') : undefined,
      }))}
    />
  );
}
