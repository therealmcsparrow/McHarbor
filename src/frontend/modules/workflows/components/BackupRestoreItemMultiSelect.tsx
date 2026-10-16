// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerBackupRestoreOptions } from '@resources/hooks/useContainerBackups';
import { MultiSelectField } from './MultiSelectField';
import type { ConfigField } from '../types';
import { assertSuccess } from '@resources/utils/api-mutation';

interface BackupRestoreItemMultiSelectProps {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
  fieldLabel: string;
  runId?: string;
  secretKey?: string;
}

/**
 * Lists what a backup run can restore — config, image, filesystem, logs
 * and each captured mount — from the run's restore-options endpoint.
 * Until a run is picked the four fixed units are offered, so an operator
 * can still pre-select "config only" on a node whose run id arrives
 * later. Values are the run item keys (`config`, `mount:/data`, ...).
 */
export function BackupRestoreItemMultiSelect({
  field,
  value,
  onChange,
  fieldLabel,
  runId,
  secretKey,
}: BackupRestoreItemMultiSelectProps) {
  const { t } = useTranslation('common');

  const { data, isLoading } = useQuery({
    queryKey: ['workflow-restore-items', runId ?? '', secretKey ? secretKey.length : 0],
    queryFn: () =>
      api
        .post<ContainerBackupRestoreOptions>(
          `/container-backups/runs/${encodeURIComponent(runId ?? '')}/restore-options`,
          secretKey?.trim() ? { secretKey: secretKey.trim() } : {},
        )
        .then(assertSuccess),
    staleTime: 30_000,
    retry: false,
    enabled: !!runId,
  });

  const staticOptions = [
    { value: 'config', label: t('workflows.restoreItemConfig') },
    { value: 'image', label: t('workflows.restoreItemImage') },
    { value: 'filesystem', label: t('workflows.restoreItemFilesystem') },
    { value: 'logs', label: t('workflows.restoreItemLogs') },
  ];

  const runOptions = (data?.items ?? []).map((item) => ({
    value: item.key,
    label: item.label,
    description: item.description,
  }));

  const options = runId && runOptions.length > 0 ? runOptions : staticOptions;

  return (
    <MultiSelectField
      field={field}
      value={value}
      onChange={onChange}
      fieldLabel={fieldLabel}
      options={options}
      isLoading={!!runId && isLoading}
    />
  );
}
