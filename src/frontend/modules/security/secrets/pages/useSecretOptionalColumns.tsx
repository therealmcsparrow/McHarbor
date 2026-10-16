// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { textColumn, boolColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { Secret } from '../types';

export function useSecretOptionalColumns() {
  const { t } = useTranslation('secrets');
  const group = t('columns.groupSecret');

  return useMemo<ColumnDef<Secret, unknown>[]>(
    () => [
      textColumn<Secret>('secretId', t('columns.id'), (r) => r.id, { group }),
      textColumn<Secret>('description', t('columns.description'), (r) => r.description, { group, width: 'xl' }),
      boolColumn<Secret>('generated', t('columns.generated'), (r) => r.generated, { group }),
      textColumn<Secret>('originModule', t('columns.originModule'), (r) => r.originModule, { group }),
      boolColumn<Secret>('autoApply', t('columns.autoApply'), (r) => r.autoApply, { group }),
      boolColumn<Secret>('system', t('columns.system'), (r) => r.system, { group }),
      textColumn<Secret>('stack', t('columns.stack'), (r) => r.stackName, { group: t('columns.groupStack') }),
      textColumn<Secret>('createdBy', t('columns.createdBy'), (r) => r.createdByName, {
        group: t('columns.groupOwner'),
      }),
      textColumn<Secret>('environment', t('columns.environment'), (r) => r.environmentName, {
        group: t('columns.groupEnvironment'),
      }),
      dateColumn<Secret>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<Secret>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
    ],
    [t, group],
  );
}
