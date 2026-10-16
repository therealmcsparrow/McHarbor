// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { textColumn, dateColumn, boolColumn } from '@resources/layout/table-page/optional-columns';
import type { BackupPlanRow } from './backup-plan-rows';

export function useBackupPlanOptionalColumns(): ColumnDef<BackupPlanRow, unknown>[] {
  const { t } = useTranslation('containers');
  const includeGroup = t('backups.plans.columnsGroupInclude');
  const group = t('backups.plans.columnsGroup');

  return useMemo(
    () => [
      boolColumn<BackupPlanRow>('includeConfig', t('backups.plans.table.includeConfig'), (r) => r.includeConfig, {
        group: includeGroup,
      }),
      boolColumn<BackupPlanRow>('includeLogs', t('backups.plans.table.includeLogs'), (r) => r.includeLogs, {
        group: includeGroup,
      }),
      boolColumn<BackupPlanRow>(
        'includeFilesystem',
        t('backups.plans.table.includeFilesystem'),
        (r) => r.includeFilesystem,
        { group: includeGroup },
      ),
      boolColumn<BackupPlanRow>('includeImage', t('backups.plans.table.includeImage'), (r) => r.includeImage, {
        group: includeGroup,
      }),
      textColumn<BackupPlanRow>('logTailLines', t('backups.plans.table.logTailLines'), (r) => r.logTailLines, {
        group,
        width: 'sm',
      }),
      dateColumn<BackupPlanRow>('lastRunAt', t('backups.plans.table.lastRunAt'), (r) => r.lastRunAt, { group }),
      textColumn<BackupPlanRow>('consistencyMode', t('backups.plans.table.consistencyMode'), (r) => r.consistencyMode, {
        group,
        width: 'sm',
      }),
      textColumn<BackupPlanRow>(
        'snapshotConfig',
        t('backups.plans.table.snapshotConfig'),
        (r) => r.snapshotConfigName,
        { group },
      ),
      dateColumn<BackupPlanRow>('createdAt', t('backups.plans.table.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<BackupPlanRow>('updatedAt', t('backups.plans.table.updatedAt'), (r) => r.updatedAt, { group }),
    ],
    [t, includeGroup, group],
  );
}
