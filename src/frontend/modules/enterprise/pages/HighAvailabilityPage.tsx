// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Grid, Stack } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PageHeader } from '@resources/layout/PageHeader';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { ClusterInstancesTable } from '../components/ClusterInstancesTable';
import { HAReadinessChecklist } from '../components/HAReadinessChecklist';
import { StartupHealthCard } from '../components/StartupHealthCard';
import { type ClusterStatus, useClusterStatus } from '../hooks/useCluster';

export default function HighAvailabilityPage() {
  const { t } = useTranslation('enterprise');
  const canView = usePermission(PERM.CLUSTER_VIEW);
  const currentStatus = useClusterStatus(canView);

  return (
    <div className="space-y-6">
      <PageHeader title={t('ha.title')} description={t('ha.description')} headingLeaf />
      {!canView ? (
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.CLUSTER_VIEW })}</Alert>
      ) : currentStatus.isLoading ? (
        <Spinner />
      ) : currentStatus.error ? (
        <Alert variant="destructive">{currentStatus.error.message}</Alert>
      ) : currentStatus.data ? (
        <ClusterView
          status={currentStatus.data}
          refresh={{ onRefresh: () => void currentStatus.refetch(), isFetching: currentStatus.isFetching }}
        />
      ) : null}
    </div>
  );
}

function ClusterView({
  status: statusValue,
  refresh,
}: {
  status: ClusterStatus;
  refresh: { onRefresh: () => void; isFetching: boolean };
}) {
  const { t } = useTranslation('enterprise');
  const conflict = statusValue.activeConflict ?? [];
  const leading = Object.entries(statusValue.roles)
    .filter(([, held]) => held)
    .map(([role]) => role)
    .sort();

  return (
    <Stack gap={6}>
      {conflict.length > 0 && (
        <Alert variant="destructive" title={t('ha.splitBrain.title')}>
          {t('ha.splitBrain.description', { nodes: conflict.join(', ') })}
        </Alert>
      )}
      {statusValue.mode === 'peer' && statusValue.databaseDriver === 'sqlite' && (
        <Alert variant="info">{t('ha.singleNode')}</Alert>
      )}
      {statusValue.mode === 'active-passive' && statusValue.pluginStore === 'local' && (
        <Alert variant="warning">{t('ha.pluginStoreLocal')}</Alert>
      )}
      <Grid columns="grid-cols-1 md:grid-cols-2 xl:grid-cols-5" gap={4}>
        <Fact label={t('ha.facts.mode')} value={t(`ha.mode.${statusValue.mode}`)} />
        <Fact label={t('ha.facts.pluginStore')} value={t(`ha.pluginStore.${statusValue.pluginStore}`)} />
        <Fact label={t('ha.facts.thisNode')} value={statusValue.nodeId}>
          <Badge variant={statusValue.role === 'active' ? 'success' : 'secondary'}>
            {t(`ha.role.${statusValue.role}`)}
          </Badge>
        </Fact>
        <Fact label={t('ha.facts.database')} value={statusValue.databaseDriver}>
          <Badge variant={statusValue.database.reachable ? 'success' : 'destructive'}>
            {t(statusValue.database.reachable ? 'ha.reachable' : 'ha.unreachable')}
          </Badge>
        </Fact>
        <Fact label={t('ha.facts.readiness')} value={t(statusValue.ready ? 'ha.ready' : 'ha.notReady')}>
          <ul className="space-y-1 text-xs">
            {statusValue.checks.map((c) => (
              <li key={c.name} className="flex items-center gap-1">
                {c.ok ? (
                  <IconCheck className="size-3 text-success" aria-hidden="true" />
                ) : (
                  <IconX className="size-3 text-destructive" aria-hidden="true" />
                )}
                {t(`ha.check.${c.name}`)}
              </li>
            ))}
          </ul>
        </Fact>
      </Grid>
      <Card>
        <CardHeader>
          <CardTitle>{t('ha.nodesTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
          <ClusterInstancesTable status={statusValue} refresh={refresh} />
        </CardContent>
      </Card>
      <HAReadinessChecklist />
      <StartupHealthCard />
      <Card>
        <CardHeader>
          <CardTitle>{t('ha.jobsTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
          <TypographyText tone="muted" size="sm">
            {t('ha.jobsDescription')}
          </TypographyText>
          <div className="mt-3 flex flex-wrap gap-2">
            {leading.length === 0 ? (
              <TypographyText size="sm">{t('ha.jobsNone')}</TypographyText>
            ) : (
              leading.map((role) => (
                <Badge key={role} variant="outline" className="font-mono">
                  {role}
                </Badge>
              ))
            )}
          </div>
        </CardContent>
      </Card>
    </Stack>
  );
}

function Fact({ label, value, children }: { label: string; value: string; children?: React.ReactNode }) {
  return (
    <Card>
      <CardContent className="space-y-2">
        <TypographyText tone="muted" size="sm">
          {label}
        </TypographyText>
        <TypographyText size="lg" weight="semibold" className="break-all">
          {value}
        </TypographyText>
        {children}
      </CardContent>
    </Card>
  );
}
