// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconFileText,
  IconPlayerPlay,
  IconPlayerStop,
  IconSortAscending,
  IconSortDescending,
  IconDownload,
} from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { LogViewer } from '@resources/components/LogViewer';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Select } from '@resources/components/ui/Select';
import { TableFilterBar } from '@resources/layout/table-page/TableFilterBar';
import { useTableSavedFilters } from '@resources/layout/table-page/useTableSavedFilters';
import { useTableViewQuery } from '@resources/layout/table-page/useTableViewApi';
import { useTableFiltersImport } from '@resources/layout/table-page/useTableViewImport';
import { useUrlSearchState } from '@resources/hooks/useUrlSearchState';
import { DegradedStateBanner } from '@resources/components/DegradedStateBanner';
import { useContainers } from '@resources/hooks/useContainers';
import { createSearchMatcher, type SearchMode } from '@resources/utils/search-filter';
import { useContainerLogs } from '../hooks/useContainerLogs';

const searchModes = ['contains', 'exact', 'regex'] as const;

export default function LogsPage() {
  const { t } = useTranslation('common');
  const { data: containers = [] } = useContainers();
  const [selectedId, setSelectedId] = useUrlSearchState<string>('', { param: 'container' });
  const [live, setLive] = useState(false);
  const [newestFirst, setNewestFirst] = useState(true);
  const [query, setQuery] = useUrlSearchState<string>('');
  const [mode, setMode] = useUrlSearchState<SearchMode>('contains', {
    param: 'mode',
    allowed: searchModes,
  });
  const {
    data: logs = '',
    isLoading,
    error,
    failureCount,
    errorUpdatedAt,
    refetch,
  } = useContainerLogs(selectedId, 500, live);
  const tableViewQuery = useTableViewQuery('logs');
  useTableFiltersImport('logs', tableViewQuery);
  const savedFilters = useTableSavedFilters('logs', {
    state: { query, mode },
    onRestore: (state) => {
      setQuery(typeof state.query === 'string' ? state.query : '');
      setMode((state.mode as SearchMode) ?? 'contains');
    },
    pristine: query === '' && mode === 'contains',
    defaults: { query: '', mode: 'contains' },
  });

  const logLines = typeof logs === 'string' ? logs.split('\n') : [];
  const orderedLines = newestFirst ? [...logLines].reverse() : logLines;
  const autoScroll = !newestFirst && live;
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const filteredLines = useMemo(() => orderedLines.filter((line) => matcher.matches(line)), [matcher, orderedLines]);

  const handleExportLogs = () => {
    const content = filteredLines.join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const container = containers.find((c) => c.Id === selectedId);
    const containerName = container?.Names?.[0]?.replace(/^\//, '') ?? selectedId.slice(0, 12);
    a.download = `${containerName}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  return (
    <div className="flex h-full flex-col space-y-4">
      <PageHeader title={t('logs.title')} description={t('logs.description')} headingLeaf />
      <DegradedStateBanner
        error={error}
        failureCount={failureCount}
        updatedAt={errorUpdatedAt}
        onRetry={() => void refetch()}
      />

      <TableFilterBar
        search={{
          value: query,
          onChange: setQuery,
          mode,
          onModeChange: setMode,
          placeholder: t('logs.searchPlaceholder'),
          error: matcher.error !== null,
        }}
        savedFilters={savedFilters}
        onClearFilters={() => {
          setQuery('');
          setMode('contains');
          savedFilters.clear();
        }}
        filters={
          <>
            <Select
              value={selectedId}
              onChange={setSelectedId}
              options={[
                { value: '', label: t('logs.selectContainerPlaceholder') },
                ...containers.map((c) => ({
                  value: c.Id,
                  label: c.Names?.[0]?.replace(/^\//, '') ?? c.Id?.slice(0, 12),
                })),
              ]}
              className="w-64"
            />
            <div className="text-xs text-muted-foreground">
              {t('filters.matchCount', { count: filteredLines.length })}
            </div>
          </>
        }
        toolbar={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant={live ? 'default' : 'outline'}
              size="sm"
              onClick={() => setLive((prev) => !prev)}
              disabled={!selectedId}
            >
              {live ? <IconPlayerStop className="h-4 w-4" /> : <IconPlayerPlay className="h-4 w-4" />}
              {live ? t('logs.liveOn') : t('logs.liveOff')}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setNewestFirst((prev) => !prev)} disabled={!selectedId}>
              {newestFirst ? <IconSortDescending className="h-4 w-4" /> : <IconSortAscending className="h-4 w-4" />}
              {newestFirst ? t('logs.newestFirst') : t('logs.oldestFirst')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleExportLogs}
              disabled={!selectedId || filteredLines.length === 0}
            >
              <IconDownload className="h-4 w-4" />
              {t('logs.export')}
            </Button>
          </div>
        }
      />

      {!selectedId ? (
        <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-border bg-background text-muted-foreground">
          <IconFileText className="mr-2 h-5 w-5" />
          {t('logs.selectContainerPrompt')}
        </div>
      ) : isLoading ? (
        <div className="min-h-0 flex-1 space-y-2 overflow-hidden rounded-lg border border-border bg-background p-4">
          {Array.from({ length: 14 }).map((_, index) => (
            <Skeleton key={`log-skeleton-${index}`} className={index % 4 === 0 ? 'h-3 w-2/3' : 'h-3 w-full'} />
          ))}
        </div>
      ) : (
        <LogViewer colorizeSeverity lines={filteredLines} autoScroll={autoScroll} className="min-h-0 flex-1" />
      )}
    </div>
  );
}
