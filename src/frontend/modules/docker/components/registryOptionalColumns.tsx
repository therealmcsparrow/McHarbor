// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { Registry } from '../hooks/useRegistries';

export function useRegistryOptionalColumns() {
  const { t } = useTranslation('docker');
  const group = t('columns.groupRegistry');

  return useMemo<ColumnDef<Registry, unknown>[]>(
    () => [
      textColumn<Registry>('registryId', t('columns.id'), (r) => r.id, { group }),
      dateColumn<Registry>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<Registry>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
    ],
    [t, group],
  );
}
