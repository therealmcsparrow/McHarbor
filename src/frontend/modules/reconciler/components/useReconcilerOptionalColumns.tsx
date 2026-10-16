// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { useEnvironmentStore } from '@resources/stores/environment';
import { textColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { DesiredState } from '../hooks/useReconciler';

export function useReconcilerOptionalColumns(): ColumnDef<DesiredState, unknown>[] {
  const { t } = useTranslation('common');
  const group = t('reconciler.columnsGroup');
  const environmentGroup = t('reconciler.columnsGroupEnvironment');
  const environments = useEnvironmentStore((s) => s.environments);
  const envNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const env of environments) map.set(env.id, env.name);
    return map;
  }, [environments]);

  return useMemo(
    () => [
      textColumn<DesiredState>('description', t('reconciler.columnDescription'), (r) => r.description, {
        group,
        width: 'xl',
      }),
      textColumn<DesiredState>('desiredStatus', t('reconciler.columnDesiredStatus'), (r) => r.desiredStatus, {
        group,
        width: 'sm',
      }),
      textColumn<DesiredState>('restartPolicy', t('reconciler.columnRestartPolicy'), (r) => r.restartPolicy, {
        group,
        width: 'sm',
      }),
      dateColumn<DesiredState>('createdAt', t('reconciler.columnCreatedAt'), (r) => r.createdAt, { group }),
      dateColumn<DesiredState>('updatedAt', t('reconciler.columnUpdatedAt'), (r) => r.updatedAt, { group }),
      textColumn<DesiredState>(
        'environment',
        t('reconciler.columnEnvironment'),
        (r) => envNameById.get(r.envId ?? ''),
        { group: environmentGroup },
      ),
    ],
    [t, group, environmentGroup, envNameById],
  );
}
