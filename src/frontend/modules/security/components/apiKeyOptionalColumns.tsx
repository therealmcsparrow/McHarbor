// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { textColumn, boolColumn, dateColumn, listColumn } from '@resources/layout/table-page/optional-columns';
import type { APIKey } from '../hooks/useAPIKeys';

export function useAPIKeyOptionalColumns() {
  const { t } = useTranslation('security');
  const group = t('columns.groupApiKey');

  return useMemo<ColumnDef<APIKey, unknown>[]>(
    () => [
      textColumn<APIKey>('keyId', t('columns.id'), (r) => r.id, { group }),
      textColumn<APIKey>('owner', t('columns.owner'), (r) => r.username, { group: t('columns.groupOwner') }),
      boolColumn<APIKey>('isActive', t('columns.isActive'), (r) => r.isActive, { group }),
      listColumn<APIKey>('environmentNames', t('columns.environments'), (r) => r.environmentNames, {
        group: t('columns.groupEnvironment'),
      }),
      dateColumn<APIKey>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<APIKey>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
    ],
    [t, group],
  );
}
