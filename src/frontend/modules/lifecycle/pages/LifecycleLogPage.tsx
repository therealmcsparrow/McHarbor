// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Select } from '@resources/components/ui/Select';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { SearchFilterToolbar } from '@resources/components/SearchFilterToolbar';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { useEnvironmentStore } from '@resources/stores/environment';
import { resolveDateRange } from '@resources/utils/date-range';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { useLifecycleEvents, type LifecycleEvent, type LifecycleSubjectType, type LifecycleSeverity } from '../hooks/useLifecycle';
import { LifecycleRow, LifecyclePagination } from '../components/LifecycleRow';


const SUBJECT_TYPES: { value: LifecycleSubjectType | ''; labelKey: string }[] = [
  { value: '', labelKey: 'lifecycle.subjectType.all' },
  { value: 'container', labelKey: 'lifecycle.subjectType.container' },
  { value: 'image', labelKey: 'lifecycle.subjectType.image' },
  { value: 'volume', labelKey: 'lifecycle.subjectType.volume' },
  { value: 'network', labelKey: 'lifecycle.subjectType.network' },
  { value: 'stack', labelKey: 'lifecycle.subjectType.stack' },
];

const SEVERITIES: { value: LifecycleSeverity | ''; labelKey: string }[] = [
  { value: '', labelKey: 'lifecycle.severity.all' },
  { value: 'success', labelKey: 'lifecycle.severity.success' },
  { value: 'info', labelKey: 'lifecycle.severity.info' },
  { value: 'warning', labelKey: 'lifecycle.severity.warning' },
  { value: 'error', labelKey: 'lifecycle.severity.error' },
];

export default function LifecycleLogPage() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const envId = useEnvironmentStore((s) => s.currentId);
  const environments = useEnvironmentStore((s) => s.environments);

  const [subjectType, setSubjectType] = useState<LifecycleSubjectType | ''>('');
  const [severity, setSeverity] = useState<LifecycleSeverity | ''>('');
  const [search, setSearch] = useState('');
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [page, setPage] = useState(1);
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);

  const resolvedRange = useMemo(
    () => resolveDateRange(dateRange.preset, dateRange.custom),
    [dateRange],
  );

  const params = useMemo(
    () => ({
      page,
      perPage: 50,
      envId: envId === '' ? undefined : envId,
      subjectType,
      severity,
      from: resolvedRange.from?.toISOString(),
      to: resolvedRange.to?.toISOString(),
      search,
    }),
    [page, envId, subjectType, severity, search, resolvedRange.from, resolvedRange.to],
  );

  const { data, isLoading, isFetching, refetch } = useLifecycleEvents(params);
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / (data?.perPage ?? 50)));
  const formatPrefs = useLocaleFormat();

  function toggleExpanded(id: string) {
    setExpanded((current) => ({ ...current, [id]: !current[id] }));
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <PageHeader
        title={t('lifecycle.title')}
        description={t('lifecycle.description')}
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            aria-label={tc('actions.refresh')}
          >
            <IconRefresh className="size-4" />
            {tc('actions.refresh')}
          </Button>
        }
      />

      <SearchFilterToolbar
        query={search}
        onQueryChange={setSearch}
        mode={'contains'}
        onModeChange={() => undefined}
        placeholder={t('lifecycle.searchPlaceholder')}
        extraControls={
          <div className="flex flex-col gap-3 md:flex-row md:items-center">
            <Select
              value={subjectType}
              onChange={(v) => {
                setSubjectType(v as LifecycleSubjectType | '');
                setPage(1);
              }}
              options={SUBJECT_TYPES.map((opt) => ({
                value: opt.value,
                label: t(opt.labelKey),
              }))}
              ariaLabel={t('lifecycle.subjectTypeLabel')}
            />
            <Select
              value={severity}
              onChange={(v) => {
                setSeverity(v as LifecycleSeverity | '');
                setPage(1);
              }}
              options={SEVERITIES.map((opt) => ({
                value: opt.value,
                label: t(opt.labelKey),
              }))}
              ariaLabel={t('lifecycle.severityLabel')}
            />
            <DateRangeFilter
              value={dateRange}
              onChange={setDateRange}
            />
          </div>
        }
      />

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
        <div className="border-b border-border bg-muted px-4 py-2 text-xs uppercase tracking-wide text-muted-foreground grid grid-cols-[auto_140px_1fr_140px_140px] items-center gap-3">
          <span className="w-4" />
          <span>{t('lifecycle.headerSubject')}</span>
          <span>{t('lifecycle.headerDetail')}</span>
          <span>{t('lifecycle.headerSeverity')}</span>
          <span className="text-right">{t('lifecycle.headerTime')}</span>
        </div>
        <div className="min-h-0 flex-1 overflow-auto">
          {isLoading && items.length === 0 ? (
            <div className="flex items-center justify-center p-12">
              <Spinner size="lg" />
            </div>
          ) : items.length === 0 ? (
            <div className="p-12 text-center text-sm text-muted-foreground">
              {t('lifecycle.empty')}
            </div>
          ) : (
            <ul className="divide-y divide-border">
              {items.map((ev: LifecycleEvent) => (
                <LifecycleRow
                  key={ev.id}
                  event={ev}
                  expanded={!!expanded[ev.id]}
                  onToggle={toggleExpanded}
                  formatPrefs={formatPrefs}
                  environments={environments}
                />
              ))}
            </ul>
          )}
        </div>
      </div>

      <LifecyclePagination
        page={page}
        totalPages={totalPages}
        total={total}
        disabled={isFetching}
        onPageChange={setPage}
        totalLabel={t('lifecycle.total')}
        pageOfLabel={t('lifecycle.pageOf')}
        previousLabel={t('lifecycle.previous')}
        nextLabel={t('lifecycle.next')}
      />
    </div>
  );
}