// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useDeferredValue, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRobot, IconTool } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Input } from '@resources/components/ui/Input';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  DateRangeFilter,
  DEFAULT_DATE_RANGE_VALUE,
  type DateRangeFilterValue,
} from '@resources/components/DateRangeFilter';
import { tableDefaultPageSize } from '@resources/components/TablePagination';
import { resolveDateRange } from '@resources/utils/date-range';
import { useAIActionTrail, useAIToolEvents } from '../hooks/useAuditCenter';
import { AITrailActionCard } from './AITrailActionCard';
import { AIToolEventsTable } from './AIToolEventsTable';
import { AuditCenterPager } from './AuditCenterPager';

function SectionTitle({ icon, title, description }: { icon: ReactNode; title: string; description: string }) {
  return (
    <div>
      <h3 className="flex items-center gap-2 text-sm font-medium text-foreground">
        {icon}
        {title}
      </h3>
      <p className="mt-1 text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

export function AuditAITrailPanel() {
  const { t } = useTranslation('audit');
  const [actor, setActor] = useState('');
  const [dateRange, setDateRange] = useState<DateRangeFilterValue>(DEFAULT_DATE_RANGE_VALUE);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<number>(tableDefaultPageSize);
  const [toolPage, setToolPage] = useState(1);
  const [toolPageSize, setToolPageSize] = useState<number>(tableDefaultPageSize);
  const range = useMemo(() => resolveDateRange(dateRange.preset, dateRange.custom), [dateRange]);
  const deferredActor = useDeferredValue(actor);

  const actions = useAIActionTrail({ page, perPage: pageSize, actor: deferredActor, range });
  const tools = useAIToolEvents({ page: toolPage, perPage: toolPageSize, actor: deferredActor, range });

  const resetPages = () => {
    setPage(1);
    setToolPage(1);
  };

  return (
    <div className="mc-glass space-y-6 rounded-lg p-6">
      <div className="flex flex-wrap items-center gap-2">
        <DateRangeFilter
          value={dateRange}
          onChange={(value) => {
            setDateRange(value);
            resetPages();
          }}
        />
        <Input
          className="w-48"
          value={actor}
          placeholder={t('filters.actorPlaceholder')}
          aria-label={t('filters.actor')}
          onChange={(evt) => {
            setActor(evt.target.value);
            resetPages();
          }}
        />
      </div>

      <section className="space-y-3">
        <SectionTitle
          icon={<IconRobot className="size-4 text-muted-foreground" aria-hidden="true" />}
          title={t('ai.actionsTitle')}
          description={t('ai.actionsDescription')}
        />
        {actions.isLoading && <Spinner size="sm" />}
        {actions.isError && (
          <Alert variant="destructive" title={t('ai.loadFailed')} description={actions.error.message} />
        )}
        {actions.data && actions.data.items.length === 0 && (
          <EmptyState icon={<IconRobot className="size-8" />} title={t('ai.empty')} variant="compact" />
        )}
        {actions.data && actions.data.items.length > 0 && (
          <ul className="space-y-2">
            {actions.data.items.map((group) => (
              <AITrailActionCard key={group.actionId} group={group} />
            ))}
          </ul>
        )}
        <AuditCenterPager
          data={actions.data}
          page={page}
          pageSize={pageSize}
          onPageChange={setPage}
          onPageSizeChange={(size) => {
            setPageSize(size);
            setPage(1);
          }}
        />
      </section>

      <section className="space-y-3">
        <SectionTitle
          icon={<IconTool className="size-4 text-muted-foreground" aria-hidden="true" />}
          title={t('ai.toolsTitle')}
          description={t('ai.toolsDescription')}
        />
        {tools.isError && <Alert variant="destructive" title={t('ai.loadFailed')} description={tools.error.message} />}
        {tools.data && tools.data.items.length === 0 && (
          <EmptyState icon={<IconTool className="size-8" />} title={t('ai.toolsEmpty')} variant="compact" />
        )}
        {tools.data && tools.data.items.length > 0 && <AIToolEventsTable rows={tools.data.items} />}
        <AuditCenterPager
          data={tools.data}
          page={toolPage}
          pageSize={toolPageSize}
          onPageChange={setToolPage}
          onPageSizeChange={(size) => {
            setToolPageSize(size);
            setToolPage(1);
          }}
        />
      </section>
    </div>
  );
}
