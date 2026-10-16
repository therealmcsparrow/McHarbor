// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { Tabs, TabsContent } from '@resources/components/ui/Tabs';
import { AuditCoveragePanel } from '../components/AuditCoveragePanel';
import { IconClipboardList } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import type { SearchMode } from '@resources/utils/search-filter';
import { EMPTY_DATE_RANGE, type DateRangePresetId } from '@resources/utils/date-range';
import { DEFAULT_DATE_RANGE_VALUE } from '@resources/components/DateRangeFilter';
import { useAuditPage, type AuditViewId } from '../hooks/useAuditPage';
import { AuditPageDialogs } from '../components/AuditPageDialogs';
import { AuditFilterControls } from '../components/AuditFilterControls';
import { AUDIT_FILTERS_DEFAULT_STATE } from '../hooks/useAuditFilters';
import { AuditCenterTabsContent, AuditHeaderActions, AuditTabsList } from '../components/AuditCenterTabs';
import { AuditVerifyDialog } from '../components/AuditVerifyDialog';

export default function AuditPage() {
  const { t } = useTranslation('common');
  const {
    filters,
    selected,
    setSelected,
    exportOpen,
    setExportOpen,
    actionsOpen,
    setActionsOpen,
    actionModuleFilter,
    setActionModuleFilter,
    verifyOpen,
    setVerifyOpen,
    isLoading,
    isFetching,
    refetch,
    total,
    totalPages,
    rangeFrom,
    rangeTo,
    envName,
    view,
    setView,
    entityTypeOptions,
    filteredEntries,
    columns,
  } = useAuditPage();

  return (
    <>
      {/* Two views, not two stacked panels. TablePage's own contract is that
          its grid is "the only scroller on the page", so rendering the coverage
          panel above it left the page unable to scroll at all. A tab gives each
          view the full height it expects, and the events table stays the
          landing view because it is what the page is for. */}
      <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
        <PageHeader
          headingLeaf
          title={t('audit.title')}
          description={t('audit.description')}
          actions={<AuditHeaderActions onVerify={() => setVerifyOpen(true)} onExport={() => setExportOpen(true)} />}
        />

        <Tabs
          value={view}
          onValueChange={(value) => setView(value as AuditViewId)}
          className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden"
        >
          <AuditTabsList />

          <TabsContent value="events" className="min-h-0 flex-1 overflow-hidden">
            <TablePage
              header={false}
              title={t('audit.title')}
              id="audit"
              search={{
                value: filters.query,
                onChange: (value) => {
                  filters.setQuery(value);
                  filters.resetPage();
                },
                mode: filters.mode,
                onModeChange: (value) => {
                  filters.setMode(value);
                  filters.resetPage();
                },
                placeholder: t('audit.searchPlaceholder'),
                error: filters.matcher.error !== null,
              }}
              savedFilters={{
                state: {
                  actionFilters: filters.actionFilters,
                  entityTypeFilter: filters.entityTypeFilter,
                  query: filters.query,
                  mode: filters.mode,
                  range: filters.dateRange.preset === 'custom' ? undefined : filters.dateRange.preset,
                  actor: filters.actor,
                  severities: filters.severities,
                },
                onRestore: (state) => {
                  filters.setActionFilters((state.actionFilters as string[]) ?? []);
                  filters.setEntityTypeFilter(typeof state.entityTypeFilter === 'string' ? state.entityTypeFilter : '');
                  filters.setQuery(typeof state.query === 'string' ? state.query : '');
                  filters.setMode((state.mode as SearchMode) ?? 'contains');
                  filters.setActor(typeof state.actor === 'string' ? state.actor : '');
                  filters.setSeverities(Array.isArray(state.severities) ? (state.severities as string[]) : []);
                  if (typeof state.range === 'string' && state.range !== 'custom') {
                    filters.setDateRange({ preset: state.range as DateRangePresetId, custom: EMPTY_DATE_RANGE });
                  } else {
                    filters.setDateRange(DEFAULT_DATE_RANGE_VALUE);
                  }
                  filters.resetPage();
                },
                pristine: filters.isPristine,
                defaults: AUDIT_FILTERS_DEFAULT_STATE,
              }}
              filters={
                <AuditFilterControls
                  dateRange={filters.dateRange}
                  onDateRangeChange={(value) => {
                    filters.setDateRange(value);
                    filters.resetPage();
                  }}
                  actionCount={filters.actionFilters.length}
                  onOpenActions={() => setActionsOpen(true)}
                  entityType={filters.entityTypeFilter}
                  onEntityTypeChange={(value) => {
                    filters.setEntityTypeFilter(value);
                    filters.resetPage();
                  }}
                  entityTypeOptions={entityTypeOptions}
                  actor={filters.actor}
                  onActorChange={(value) => {
                    filters.setActor(value);
                    filters.resetPage();
                  }}
                  severities={filters.severities}
                  onSeveritiesChange={(value) => {
                    filters.setSeverities(value);
                    filters.resetPage();
                  }}
                />
              }
              toolbar={{
                columns: true,
                density: true,
                refresh: { onRefresh: () => void refetch(), isFetching },
                // Only the server endpoint can reach past the loaded page.
                export: { filename: 'mcharbor-audit-export', server: () => setExportOpen(true) },
                chips: { items: filters.chips, onClear: filters.clearAll },
              }}
              columns={columns}
              data={filteredEntries}
              loading={isLoading}
              empty={{
                icon: <IconClipboardList className="size-8" />,
                title: filters.query.trim() ? t('filters.noMatches') : t('audit.noEntries'),
              }}
              getRowId={(entry) => entry.id}
              pagination={{
                mode: 'server',
                page: filters.page,
                totalPages,
                pageSize: filters.pageSize,
                total,
                from: rangeFrom,
                to: rangeTo,
                onPageChange: (next) => filters.clampPage(next, totalPages),
                onPageSizeChange: (next) => {
                  filters.setPageSize(next);
                  filters.setPage(1);
                },
              }}
            />
          </TabsContent>

          <AuditCenterTabsContent envName={envName} />

          <TabsContent value="coverage" className="min-h-0 flex-1 overflow-auto">
            <AuditCoveragePanel />
          </TabsContent>
        </Tabs>
      </div>

      <AuditPageDialogs
        filters={filters}
        selected={selected}
        setSelected={setSelected}
        exportOpen={exportOpen}
        setExportOpen={setExportOpen}
        actionsOpen={actionsOpen}
        setActionsOpen={setActionsOpen}
        actionModuleFilter={actionModuleFilter}
        setActionModuleFilter={setActionModuleFilter}
        envName={envName}
      />
      <AuditVerifyDialog open={verifyOpen} onOpenChange={setVerifyOpen} />
    </>
  );
}
