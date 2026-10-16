// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useDeferredValue, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import { resolveDateRange } from '@resources/utils/date-range';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useAuditDenials } from '../hooks/useAuditCenter';
import { AuditDenialsTable } from './AuditDenialsTable';
import { AuditCenterPager } from './AuditCenterPager';

type AuditDenialsPanelProps = {
  envName: (id: string | null, storedName?: string | null) => string;
};

export function AuditDenialsPanel({ envName }: AuditDenialsPanelProps) {
  const { t } = useTranslation('audit');
  const environments = useEnvironmentStore((s) => s.environments);
  const [actor, setActor] = useState('');
  const [permission, setPermission] = useState('');
  const [environment, setEnvironment] = useState('');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<number>(tableDefaultPageSize);
  const range = useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);
  const deferredActor = useDeferredValue(actor);
  const deferredPermission = useDeferredValue(permission);

  const { data, isLoading, isError, error } = useAuditDenials({
    page,
    perPage: pageSize,
    actor: deferredActor,
    permission: deferredPermission,
    environment,
    range,
  });

  const environmentOptions = useMemo(
    () => [
      { value: '', label: t('filters.anyEnvironment') },
      ...environments.map((env) => ({ value: env.id, label: env.name })),
    ],
    [environments, t],
  );

  return (
    <div className="mc-glass space-y-4 rounded-lg p-6">
      <div>
        <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
          <IconLock className="size-4 text-muted-foreground" aria-hidden="true" />
          {t('denials.title')}
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">{t('denials.description')}</p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <DateRangeFilter
          value={dateRange}
          onChange={(value) => {
            setDateRange(value);
            setPage(1);
          }}
        />
        <Input
          className="w-48"
          value={actor}
          placeholder={t('filters.actorPlaceholder')}
          aria-label={t('filters.actor')}
          onChange={(eventItem) => {
            setActor(eventItem.target.value);
            setPage(1);
          }}
        />
        <Input
          className="w-64"
          value={permission}
          placeholder={t('filters.permissionPlaceholder')}
          aria-label={t('filters.permission')}
          onChange={(evt) => {
            setPermission(evt.target.value);
            setPage(1);
          }}
        />
        <Select
          value={environment}
          onChange={(value) => {
            setEnvironment(value);
            setPage(1);
          }}
          options={environmentOptions}
          placeholder={t('filters.environment')}
          className="w-48"
        />
      </div>
      {isLoading && <Spinner size="sm" />}
      {isError && <Alert variant="destructive" title={t('denials.loadFailed')} description={error.message} />}
      {data && data.items.length === 0 && (
        <EmptyState icon={<IconLock className="size-8" />} title={t('denials.empty')} variant="compact" />
      )}
      {data && data.items.length > 0 && <AuditDenialsTable rows={data.items} envName={envName} />}
      <AuditCenterPager
        data={data}
        page={page}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={(size) => {
          setPageSize(size);
          setPage(1);
        }}
      />
    </div>
  );
}
