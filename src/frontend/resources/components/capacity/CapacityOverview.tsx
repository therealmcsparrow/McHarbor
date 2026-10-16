// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { IconGauge } from '@tabler/icons-react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { useCapacity } from '@resources/hooks/useCapacity';
import { CapacitySentence, capacityKindKey } from './CapacitySentence';

type CapacityOverviewProps = {
  envId?: string;
};

export function CapacityOverview({ envId }: CapacityOverviewProps) {
  const { t } = useTranslation('host');
  const allowed = usePermission(PERM.HOST_VIEW);
  const { data, isLoading, error } = useCapacity(envId, allowed);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('capacity.title')}</CardTitle>
        <CardDescription>
          {data
            ? t('capacity.window', { window: data.windowDays, required: data.requiredDays, ceiling: data.redPercent })
            : t('capacity.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {!allowed ? (
          <p className="text-sm text-muted-foreground">{t('capacity.denied', { permission: PERM.HOST_VIEW })}</p>
        ) : isLoading ? (
          <div className="flex h-24 items-center justify-center">
            <Spinner />
          </div>
        ) : error ? (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('capacity.loadFailed')}</span>
            <StatusError error={error instanceof Error ? error.message : String(error)} title={t('capacity.title')} />
          </div>
        ) : !data || data.subjects.length === 0 ? (
          <EmptyState
            icon={<IconGauge className="size-6" aria-hidden="true" />}
            title={t('capacity.empty.title')}
            description={t('capacity.empty.description')}
          />
        ) : (
          <ul className="divide-y divide-border">
            {data.subjects.map((subject) => (
              <li key={`${subject.subjectKind}:${subject.subjectId}`} className="py-3 first:pt-0 last:pb-0">
                <p className="text-xs tracking-wider text-muted-foreground uppercase">
                  {t(capacityKindKey(subject.subjectKind))}
                </p>
                <CapacitySentence subject={subject} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
