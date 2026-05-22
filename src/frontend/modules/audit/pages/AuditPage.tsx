// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconClipboardList, IconEye } from '@tabler/icons-react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PageHeader } from '@resources/layout/PageHeader';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { Select } from '@resources/components/ui/Select';
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
  type AuditEntry,
  AuditEntryDetails,
  ACTION_VARIANTS,
} from '../components/AuditEntryDetails';
import { useAuditLogs } from '../hooks/useAuditLogs';

export default function AuditPage() {
  const { t } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);
  const [actionFilter, setActionFilter] = useState('');
  const [entityTypeFilter, setEntityTypeFilter] = useState('');
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [selected, setSelected] = useState<AuditEntry | null>(null);
  const { data: entries = [], isLoading } = useAuditLogs(actionFilter, entityTypeFilter);
  const matcher = useMemo(() => createSearchMatcher(query, mode), [mode, query]);
  const savedFilters = useSavedSearchFilters<{
    actionFilter: string;
    entityTypeFilter: string;
    query: string;
    mode: SearchMode;
  }>('mcharbor-audit-filters');

  const envName = useCallback((id: string | null) => {
    if (!id) return t('audit.local');
    return environments.find((e) => e.id === id)?.name ?? truncateId(id);
  }, [environments, t]);

  const { actionOptions, entityTypeOptions } = useMemo(() => {
    const actions = new Set<string>();
    const types = new Set<string>();
    for (const entry of entries) {
      if (entry.action) actions.add(entry.action);
      if (entry.entityType) types.add(entry.entityType);
    }
    return {
      actionOptions: [
        { value: '', label: t('audit.allActions') },
        ...[...actions].sort().map((a) => ({ value: a, label: a })),
      ],
      entityTypeOptions: [
        { value: '', label: t('audit.allEntityTypes') },
        ...[...types].sort().map((et) => ({ value: et, label: et })),
      ],
    };
  }, [entries, t]);

  const filteredEntries = useMemo(
    () =>
      entries.filter((entry) =>
        matchesSearchFields(query, mode, [
          formatDate(entry.timestamp),
          entry.username,
          entry.action,
          entry.entityName,
          entry.entityType,
          entry.entityId,
          envName(entry.environmentId),
          entry.details,
        ]).matched),
    [entries, envName, mode, query],
  );

  const handleSaveFilter = () => {
    const label = window.prompt(t('filters.savePrompt'));
    if (!label) {
      return;
    }

    savedFilters.savePreset(label, { actionFilter, entityTypeFilter, query, mode });
  };

  return (
    <div className="space-y-6">
      <PageHeader title={t('audit.title')} description={t('audit.description')} />
      <SearchFilterToolbar
        query={query}
        onQueryChange={setQuery}
        mode={mode}
        onModeChange={setMode}
        placeholder={t('audit.searchPlaceholder')}
        regexError={matcher.error !== null}
        savedFilters={savedFilters.presets.map((preset) => ({ value: preset.id, label: preset.label }))}
        selectedSavedFilterId={savedFilters.selectedPresetId}
        onSavedFilterSelect={(value) => {
          savedFilters.setSelectedPresetId(value);
          const preset = savedFilters.presets.find((entry) => entry.id === value);
          if (!preset) {
            return;
          }
          setActionFilter(preset.state.actionFilter);
          setEntityTypeFilter(preset.state.entityTypeFilter);
          setQuery(preset.state.query);
          setMode(preset.state.mode);
        }}
        onSaveFilter={handleSaveFilter}
        onDeleteSavedFilter={savedFilters.deleteSelectedPreset}
        extraControls={
          <div className="flex flex-col gap-3 md:flex-row md:items-center">
            <Select value={actionFilter} onChange={setActionFilter} options={actionOptions} placeholder={t('audit.filterAction')} className="w-48" />
            <Select value={entityTypeFilter} onChange={setEntityTypeFilter} options={entityTypeOptions} placeholder={t('audit.filterEntityType')} className="w-48" />
            <div className="text-xs text-muted-foreground">{t('filters.matchCount', { count: filteredEntries.length })}</div>
          </div>
        }
      />

      {isLoading ? (
        <div className="flex h-64 items-center justify-center">
          <Spinner size="lg" />
        </div>
      ) : filteredEntries.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
          <IconClipboardList className="mb-2 h-8 w-8" />
          <p>{query.trim() ? t('filters.noMatches') : t('audit.noEntries')}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnTimestamp')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnUser')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnAction')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnEntity')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnEnvironment')}</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('audit.columnDetails')}</th>
                <th className="px-4 py-2.5 text-right font-medium text-muted-foreground" />
              </tr>
            </thead>
            <tbody>
              {filteredEntries.map((entry) => (
                <tr key={entry.id} className="border-b border-border last:border-0 transition-colors hover:bg-muted/30">
                  <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap">{formatDate(entry.timestamp)}</td>
                  <td className="px-4 py-2.5 font-medium text-foreground">{entry.username ?? '-'}</td>
                  <td className="px-4 py-2.5">
                    <Badge variant={ACTION_VARIANTS[entry.action] ?? 'secondary'}>{entry.action}</Badge>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex flex-col">
                      <span className="font-medium text-foreground truncate max-w-48">{entry.entityName ?? entry.entityType ?? '-'}</span>
                      {entry.entityId && <span className="font-mono text-xs text-muted-foreground">{truncateId(entry.entityId)}</span>}
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">{envName(entry.environmentId)}</td>
                  <td className="px-4 py-2.5 text-muted-foreground max-w-48 truncate">{entry.details ?? '-'}</td>
                  <td className="px-4 py-2.5 text-right">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="outline" size="icon-sm" onClick={() => setSelected(entry)} aria-label={t('audit.viewDetails')}>
                          <IconEye className="size-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t('audit.viewDetails')}</TooltipContent>
                    </Tooltip>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Dialog open={!!selected} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-w-2xl p-0">
          <DialogHeader>
            <DialogTitle>{t('audit.detailsTitle')}</DialogTitle>
            <DialogDescription>
              {selected?.action} — {selected?.entityName ?? selected?.entityType ?? ''}
            </DialogDescription>
          </DialogHeader>
          {selected && (
            <DialogBody className="p-0">
              <AuditEntryDetails entry={selected} envName={envName} />
            </DialogBody>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

