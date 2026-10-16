// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { textColumn, dateColumn, boolColumn } from '@resources/layout/table-page/optional-columns';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';

export function useEnvironmentOptionalColumns() {
  const { t } = useTranslation('environments');
  const group = t('columns.groupEnvironment');
  const agent = t('columns.groupAgent');

  return useMemo<ColumnDef<EnvironmentListItem, unknown>[]>(
    () => [
      textColumn<EnvironmentListItem>('envId', t('columns.id'), (r) => r.id, { group }),
      textColumn<EnvironmentListItem>('stackCount', t('columns.stackCount'), (r) => r.stackCount, {
        group,
        width: 'xs',
      }),
      boolColumn<EnvironmentListItem>('isActive', t('columns.isActive'), (r) => r.isActive, { group }),
      dateColumn<EnvironmentListItem>('lastConnected', t('columns.lastConnected'), (r) => r.lastConnected, { group }),
      textColumn<EnvironmentListItem>('timezone', t('columns.timezone'), (r) => r.timezone, { group }),
      textColumn<EnvironmentListItem>('agentStatus', t('columns.agentStatus'), (r) => r.agentStatus, { group: agent }),
      textColumn<EnvironmentListItem>('agentBuild', t('columns.agentBuild'), (r) => r.agentBuild, { group: agent }),
      textColumn<EnvironmentListItem>(
        'agentPlatform',
        t('columns.agentPlatform'),
        (r) => (r.agentOs ? `${r.agentOs}/${r.agentArch ?? ''}` : null),
        { group: agent },
      ),
      dateColumn<EnvironmentListItem>('agentLastSeen', t('columns.agentLastSeen'), (r) => r.agentLastSeen, {
        group: agent,
      }),
      textColumn<EnvironmentListItem>('agentInstallKind', t('columns.agentInstallKind'), (r) => r.agentInstallKind, {
        group: agent,
      }),
      textColumn<EnvironmentListItem>(
        'ssh',
        t('columns.ssh'),
        (r) => (r.sshHost ? `${r.sshUser ?? ''}@${r.sshHost}:${r.sshPort ?? 22}` : null),
        { group: t('columns.groupConnection') },
      ),
      dateColumn<EnvironmentListItem>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<EnvironmentListItem>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
      textColumn<EnvironmentListItem>('logRetentionDays', t('columns.logRetentionDays'), (r) => r.logRetentionDays, {
        group: t('columns.groupMaintenance'),
        width: 'sm',
      }),
      textColumn<EnvironmentListItem>(
        'metricRetentionHours',
        t('columns.metricRetentionHours'),
        (r) => r.metricRetentionHours,
        { group: t('columns.groupMaintenance'), width: 'sm' },
      ),
      textColumn<EnvironmentListItem>(
        'autoUpdateWindow',
        t('columns.autoUpdateWindow'),
        (r) => (r.autoUpdateWindowStart ? `${r.autoUpdateWindowStart}–${r.autoUpdateWindowEnd ?? ''}` : null),
        { group: t('columns.groupMaintenance') },
      ),
      boolColumn<EnvironmentListItem>(
        'containerPruneEnabled',
        t('columns.containerPruneEnabled'),
        (r) => r.containerPruneEnabled,
        { group: t('columns.groupMaintenance') },
      ),
      textColumn<EnvironmentListItem>(
        'containerPruneStoppedDays',
        t('columns.containerPruneStoppedDays'),
        (r) => r.containerPruneStoppedDays,
        { group: t('columns.groupMaintenance'), width: 'sm' },
      ),
      boolColumn<EnvironmentListItem>(
        'imagePruneDanglingOnly',
        t('columns.imagePruneDanglingOnly'),
        (r) => r.imagePruneDanglingOnly,
        { group: t('columns.groupMaintenance') },
      ),
      boolColumn<EnvironmentListItem>(
        'automaticImagePruning',
        t('columns.automaticImagePruning'),
        (r) => r.automaticImagePruningEnabled,
        { group: t('columns.groupMaintenance') },
      ),
    ],
    [t, group, agent],
  );
}
