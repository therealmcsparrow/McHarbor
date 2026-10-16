// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCamera, IconCircleCheck, IconExclamationCircle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { ListItem } from '@resources/components/ui/ListItem';
import type { RecentSnapshot } from '../hooks/useSnapshotConfigs';

type RecentSnapshotsCardProps = {
  noEnv: boolean;
  recent: {
    data?: RecentSnapshot[];
    isLoading: boolean;
    isError: boolean;
    error: unknown;
  };
};

/** The last snapshots taken on this environment, newest first. */
export function RecentSnapshotsCard({ noEnv, recent }: RecentSnapshotsCardProps) {
  const { t } = useTranslation('settings');
  const rows = recent.data ?? [];

  return (
    <Card>
      <CardHeader>
        <h3 className="text-lg font-semibold">{t('snapshots.recent.title')}</h3>
        <p className="text-sm text-muted-foreground">{t('snapshots.recent.subtitle')}</p>
      </CardHeader>
      <CardContent>
        {noEnv && <EmptyState title={t('snapshots.recent.noEnvironments')} />}
        {!noEnv && recent.isLoading && (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        )}
        {!noEnv && recent.isError && (
          <ErrorState error={recent.error as Error} title={t('snapshots.recent.loadingFailed')} />
        )}
        {!noEnv && !recent.isLoading && rows.length === 0 && (
          <EmptyState title={t('snapshots.recent.empty')} description={t('snapshots.recent.emptyHint')} />
        )}
        {!noEnv && rows.length > 0 && (
          <div className="divide-y divide-border">
            {rows.map((row) => (
              <ListItem
                key={row.runId}
                title={row.snapshotName}
                subtitle={
                  `${row.containerName ?? row.runId} · ${row.snapshotDriver}` +
                  (row.snapshotMountPath ? ` · ${row.snapshotMountPath}` : '')
                }
                badge={
                  row.status === 'success' ? (
                    <Badge variant="default">
                      <IconCircleCheck className="size-3" />
                    </Badge>
                  ) : row.status === 'failure' ? (
                    <Badge variant="destructive">
                      <IconExclamationCircle className="size-3" />
                    </Badge>
                  ) : (
                    <Badge variant="secondary">
                      <IconCamera className="size-3" />
                    </Badge>
                  )
                }
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
