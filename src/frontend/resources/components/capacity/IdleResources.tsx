// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { IconSparkles } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatBytes } from '@resources/utils/format';
import { useIdleResources, type IdleResource } from '@resources/hooks/useCapacity';

type IdleResourcesProps = {
  envId?: string;
  days?: number;
};

function SizeCell({ row }: { row: IdleResource }) {
  const { t } = useTranslation('host');
  if (!row.sizeKnown) {
    return (
      <span className="text-muted-foreground">{t(`capacity.idle.sizeUnknown.${row.sizeReason ?? 'not_sampled'}`)}</span>
    );
  }
  return <span>{formatBytes(row.sizeRwBytes)}</span>;
}

export function IdleResources({ envId, days }: IdleResourcesProps) {
  const { t } = useTranslation('host');
  const allowed = usePermission(PERM.HOST_VIEW);
  const { data, isLoading, error } = useIdleResources(envId, days, allowed);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('capacity.idle.title')}</CardTitle>
        <CardDescription>
          {data
            ? t('capacity.idle.description', {
                days: data.thresholdDays,
                reclaimable: formatBytes(data.reclaimableBytes),
              })
            : t('capacity.idle.descriptionPlain')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {data?.sizesUnavailable && <Alert variant="info">{t('capacity.idle.sizesUnavailable')}</Alert>}
        {!allowed ? (
          <p className="text-sm text-muted-foreground">{t('capacity.denied', { permission: PERM.HOST_VIEW })}</p>
        ) : isLoading ? (
          <div className="flex h-24 items-center justify-center">
            <Spinner />
          </div>
        ) : error ? (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('capacity.idle.loadFailed')}</span>
            <StatusError
              error={error instanceof Error ? error.message : String(error)}
              title={t('capacity.idle.title')}
            />
          </div>
        ) : !data || data.containers.length === 0 ? (
          <EmptyState
            icon={<IconSparkles className="size-6" aria-hidden="true" />}
            title={t('capacity.idle.empty.title')}
            description={t('capacity.idle.empty.description')}
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('capacity.idle.columns.container')}</TableHead>
                <TableHead>{t('capacity.idle.columns.state')}</TableHead>
                <TableHead>{t('capacity.idle.columns.forHowLong')}</TableHead>
                <TableHead>{t('capacity.idle.columns.holds')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.containers.map((row) => (
                <TableRow key={`${row.environmentId}:${row.containerId}`}>
                  <TableCell className="font-medium">{row.containerName || row.containerId}</TableCell>
                  <TableCell>
                    <Badge variant={row.reason === 'stopped' ? 'neutral' : 'warning'}>
                      {t(`capacity.idle.reason.${row.reason}`)}
                    </Badge>
                  </TableCell>
                  <TableCell>{t('capacity.idle.days', { days: row.idleDays })}</TableCell>
                  <TableCell>
                    <SizeCell row={row} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
