// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerBackupOptions } from '@resources/hooks/useContainerBackups';
import { MultiSelectField } from './MultiSelectField';
import type { ConfigField } from '../types';
import { assertSuccess } from '@resources/utils/api-mutation';

interface ContainerMountMultiSelectProps {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
  fieldLabel: string;
  envId?: string;
  containerId?: string;
}

const MOUNT_PREFIX = 'mount:';

/**
 * Lists the volume and bind mounts of the selected container, read from
 * the same backup-options endpoint the container backup dialog uses.
 * Stored values are the mount destinations without the `mount:` key
 * prefix — exactly what `selected_mounts` on a backup plan holds.
 */
export function ContainerMountMultiSelect({
  field,
  value,
  onChange,
  fieldLabel,
  envId,
  containerId,
}: ContainerMountMultiSelectProps) {
  const { t } = useTranslation('common');

  const { data, isLoading } = useQuery({
    queryKey: ['container-backup-options', envId ?? '', containerId ?? ''],
    queryFn: () =>
      api
        .get<ContainerBackupOptions>(
          `/containers/${encodeURIComponent(containerId ?? '')}/backups/options`,
          envId ? { env: envId } : {},
        )
        .then(assertSuccess),
    staleTime: 30_000,
    enabled: !!envId && !!containerId,
  });

  const options = (data?.options ?? [])
    .filter((option) => option.type === 'volume' || option.type === 'bind')
    .map((option) => ({
      value: option.key.startsWith(MOUNT_PREFIX) ? option.key.slice(MOUNT_PREFIX.length) : option.key,
      label: option.label,
      description: option.description,
    }));

  const blockedLabel = !envId
    ? t('workflows.selectEnvironmentFirst')
    : !containerId
      ? t('workflows.selectContainerFirst')
      : !isLoading && options.length === 0
        ? t('workflows.noMountsFound')
        : undefined;

  return (
    <MultiSelectField
      field={field}
      value={value}
      onChange={onChange}
      fieldLabel={fieldLabel}
      options={options}
      blockedLabel={blockedLabel}
      isLoading={isLoading}
    />
  );
}
