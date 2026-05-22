// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileText, IconPlayerPlay, IconPlayerStop, IconSortAscending, IconSortDescending, IconDownload } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { LogViewer } from '@resources/components/LogViewer';
import { Spinner } from '@resources/components/ui/Spinner';
import { SearchFilterToolbar } from '@resources/components/SearchFilterToolbar';
import { useSavedSearchFilters } from '@resources/hooks/useSavedSearchFilters';
import { createSearchMatcher, type SearchMode } from '@resources/utils/search-filter';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { useContainerLogs } from '../hooks/useContainerLogs';

export default function LogsPage() {
  const { t } = useTranslation('common');
  const { data: containers = [] } = useContainers();
  const [selectedId, setSelectedId] = useState('');
  const [live, setLive] = useState(false);
  const [newestFirst, setNewestFirst] = useState(true);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const { data: logs = '', isLoading } = useContainerLogs(selectedId, 500, live);
  const savedFilters = useSavedSearchFilters<{ query: string; mode: SearchMode }>('mcharbor-logs-filters');

  const logLines = typeof logs === 'string' ? logs.split('\n') : [];
  const orderedLines = newestFirst ? [...logLines].reverse() : logLines;
  const autoScroll = !newestFirst && live;
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const filteredLines = useMemo(
    () => orderedLines.filter((line) => matcher.matches(line)),
    [matcher, orderedLines],
  );

  const handleExportLogs = () => {
    const content = filteredLines.join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const container = containers.find((c) => c.Id === selectedId);
    const name = container?.Names?.[0]?.replace(/^\//, '') ?? selectedId.slice(0, 12);
    a.download = `${name}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const handleSaveFilter = () => {
    const label = window.prompt(t('filters.savePrompt'));
    if (!label) {
      return;
    }

    savedFilters.savePreset(label, { query, mode });
  };

  return (
    <div className="flex h-full flex-col space-y-4">
      <PageHeader title={t('logs.title')} description={t('logs.description')} />

      <SearchFilterToolbar
        query={query}
        onQueryChange={setQuery}
        mode={mode}
        onModeChange={setMode}
        placeholder={t('logs.searchPlaceholder')}
        regexError={matcher.error !== null}
        savedFilters={savedFilters.presets.map((preset) => ({ value: preset.id, label: preset.label }))}
        selectedSavedFilterId={savedFilters.selectedPresetId}
        onSavedFilterSelect={(value) => {
          savedFilters.setSelectedPresetId(value);
          const preset = savedFilters.presets.find((entry) => entry.id === value);
          if (!preset) {
            return;
          }
          setQuery(preset.state.query);
          setMode(preset.state.mode);
        }}
        onSaveFilter={handleSaveFilter}
        onDeleteSavedFilter={savedFilters.deleteSelectedPreset}
        extraControls={
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex items-center gap-3">
              <select
                value={selectedId}
                onChange={(e) => setSelectedId(e.target.value)}
                className="w-64 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
              >
                <option value="">{t('logs.selectContainerPlaceholder')}</option>
                {containers.map((c) => (
                  <option key={c.Id} value={c.Id}>
                    {c.Names?.[0]?.replace(/^\//, '') ?? c.Id?.slice(0, 12)}
                  </option>
                ))}
              </select>
              <div className="text-xs text-muted-foreground">
                {t('filters.matchCount', { count: filteredLines.length })}
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-3">
              <Button
                variant={live ? 'default' : 'outline'}
                size="sm"
                onClick={() => setLive((prev) => !prev)}
                disabled={!selectedId}
              >
                {live ? <IconPlayerStop className="h-4 w-4" /> : <IconPlayerPlay className="h-4 w-4" />}
                {live ? t('logs.liveOn') : t('logs.liveOff')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setNewestFirst((prev) => !prev)}
                disabled={!selectedId}
              >
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
          </div>
        }
      />

      {!selectedId ? (
        <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-border bg-[#0a0a0a] text-muted-foreground">
          <IconFileText className="mr-2 h-5 w-5" />
          {t('logs.selectContainerPrompt')}
        </div>
      ) : isLoading ? (
        <div className="flex min-h-0 flex-1 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : (
        <LogViewer
          lines={filteredLines}
          autoScroll={autoScroll}
          className="min-h-0 flex-1"
        />
      )}
    </div>
  );
}
