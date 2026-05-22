// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconActivity, IconEye } from '@tabler/icons-react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PageHeader } from '@resources/layout/PageHeader';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { SearchFilterToolbar } from '@resources/components/SearchFilterToolbar';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { formatDate, truncateId } from '@resources/utils/format';
import { useSavedSearchFilters } from '@resources/hooks/useSavedSearchFilters';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import {
  type ContainerEvent,
  EventDetails,
  parseMeta,
  ACTION_VARIANTS,
} from '../components/EventDetails';
import { useEvents } from '../hooks/useEvents';

export default function ActivityPage() {
  const { t } = useTranslation('common');
  const { data: events = [], isLoading } = useEvents();
  const environments = useEnvironmentStore((s) => s.environments);
  const [selected, setSelected] = useState<ContainerEvent | null>(null);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const savedFilters = useSavedSearchFilters<{ query: string; mode: SearchMode }>('mcharbor-activity-filters');

  const envName = useCallback((id: string | null) => {
    if (!id) return 'Local';
    return environments.find((e) => e.id === id)?.name ?? truncateId(id);
  }, [environments]);

  const filteredEvents = useMemo(
    () =>
      events.filter((event) =>
        matchesSearchFields(query, mode, [
          formatDate(event.timestamp),
          envName(event.environmentId),
          event.action,
          event.containerName,
          event.containerId,
          parseMeta(event.metadata).image,
        ]).matched),
    [envName, events, mode, query],
  );

  const handleSaveFilter = () => {
    const label = window.prompt(t('filters.savePrompt'));
    if (!label) {
      return;
    }

    savedFilters.savePreset(label, { query, mode });
  };

  return (
    <div className="space-y-6">
      <PageHeader title={t('activity.title')} description={t('activity.description')} />
      <SearchFilterToolbar
        query={query}
        onQueryChange={setQuery}
        mode={mode}
        onModeChange={setMode}
        placeholder={t('activity.searchPlaceholder')}
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
        extraControls={<div className="text-xs text-muted-foreground">{t('filters.matchCount', { count: filteredEvents.length })}</div>}
      />

      {isLoading ? (
        <div className="flex h-64 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : filteredEvents.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
          <IconActivity className="mb-2 h-8 w-8" />
          <p>{query.trim() ? t('filters.noMatches') : t('activity.noEvents')}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnTimestamp')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnEnvironment')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnAction')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnContainer')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnImage')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('activity.columnExitCode')}</th>
                <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">{t('activity.columnActions')}</th>
              </tr>
            </thead>
            <tbody>
              {filteredEvents.map((event) => {
                const meta = parseMeta(event.metadata);
                const image = meta.image ?? null;
                const exitCode = meta.exitCode ?? null;

                return (
                  <tr key={event.id} className="border-b border-border last:border-0 transition-colors hover:bg-muted/30">
                    <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap">{formatDate(event.timestamp)}</td>
                    <td className="px-4 py-2.5 text-foreground">{envName(event.environmentId)}</td>
                    <td className="px-4 py-2.5">
                      <Badge variant={ACTION_VARIANTS[event.action] ?? 'secondary'}>{event.action}</Badge>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex flex-col">
                        <span className="font-medium text-foreground truncate max-w-48">{event.containerName ?? truncateId(event.containerId)}</span>
                        <span className="font-mono text-xs text-muted-foreground">{truncateId(event.containerId)}</span>
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-muted-foreground font-mono text-xs truncate max-w-48">{image ?? '-'}</td>
                    <td className="px-4 py-2.5">
                      {exitCode != null ? (
                        <Badge variant={exitCode === '0' ? 'secondary' : 'destructive'}>{exitCode}</Badge>
                      ) : (
                        <span className="text-muted-foreground">-</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button variant="outline" size="icon-sm" onClick={() => setSelected(event)} aria-label={t('activity.viewDetails')}>
                            <IconEye className="size-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{t('activity.viewDetails')}</TooltipContent>
                      </Tooltip>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <Dialog open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-w-2xl p-0">
          <DialogHeader>
            <DialogTitle>{t('activity.detailsTitle')}</DialogTitle>
            <DialogDescription>
              {selected?.containerName ?? truncateId(selected?.containerId)} — {selected?.action}
            </DialogDescription>
          </DialogHeader>
          {selected && (
            <DialogBody className="p-0">
              <EventDetails event={selected} envName={envName} />
            </DialogBody>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

