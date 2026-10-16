// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Command } from 'cmdk';
import { IconSearch, IconLoader2 } from '@tabler/icons-react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { Select } from '@resources/components/ui/Select';
import { Dialog, DialogBody, DialogContent, DialogDescription, DialogTitle } from '@resources/components/ui/Dialog';
import { type SearchMode } from '@resources/utils/search-filter';
import { GlobalSearchCommandGroups } from './GlobalSearchCommandGroups';
import { GlobalSearchResourceGroups } from './GlobalSearchResourceGroups';
import { useGlobalSearchData } from './useGlobalSearchData';
import { useGlobalSearchResults, type RegisteredCommand } from './useGlobalSearchResults';

const ESC_KEY_LABEL = 'esc';

export type GlobalSearchProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function GlobalSearch({ open: isOpen, onOpenChange }: GlobalSearchProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const environments = useEnvironmentStore((s) => s.environments);
  const setCurrentEnvironment = useEnvironmentStore((s) => s.setCurrentId);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');

  const data = useGlobalSearchData(environments, isOpen);
  const results = useGlobalSearchResults(query, mode, data);
  const { matcher } = results;

  if (!isOpen && (query !== '' || mode !== 'contains')) {
    setQuery('');
    setMode('contains');
  }

  // A plugin page reads the topbar environment, not `?env=`, so an item from another
  // environment switches to it first, as a host detail link's `?env=` does.
  const select = (path: string, envId?: string) => {
    if (envId) setCurrentEnvironment(envId);
    void navigate(path);
    onOpenChange(false);
  };

  const selectRegisteredCommand = (command: RegisteredCommand) => {
    if (command.action) {
      onOpenChange(false);
      requestAnimationFrame(command.action);
      return;
    }
    void navigate(command.to);
    onOpenChange(false);
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        className="top-[10vh] max-h-[80vh] w-[calc(100%-2rem)] max-w-2xl translate-y-0 p-0"
        onOpenAutoFocus={() => {
          returnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(evt) => {
          const target = returnFocusRef.current;
          if (!target?.isConnected) return;
          evt.preventDefault();
          requestAnimationFrame(() => target.focus());
        }}
      >
        <DialogTitle className="sr-only">{t('globalSearch.title')}</DialogTitle>
        <DialogDescription className="sr-only">{t('globalSearch.description')}</DialogDescription>
        <DialogBody className="overflow-hidden p-0">
          <Command className="flex flex-col" shouldFilter={false}>
            <div className="flex items-center border-b border-hairline pr-12">
              <IconSearch className="ml-4 h-4 w-4 shrink-0 text-muted-foreground" />
              <Command.Input
                value={query}
                onValueChange={setQuery}
                placeholder={t('globalSearch.placeholder')}
                className="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
              />
              {data.isLoading && <IconLoader2 className="mr-4 h-4 w-4 shrink-0 animate-spin text-muted-foreground" />}
            </div>
            <div className="border-b border-hairline px-3 py-2">
              <Select
                value={mode}
                onChange={(value) => setMode(value as SearchMode)}
                options={[
                  { value: 'contains', label: t('filters.modeContains') },
                  { value: 'exact', label: t('filters.modeExact') },
                  { value: 'regex', label: t('filters.modeRegex') },
                ]}
                searchable={false}
                variant="outline"
                className="w-40"
              />
              {matcher.error && <p className="mt-2 text-xs text-destructive">{t('filters.invalidRegex')}</p>}
            </div>

            <Command.List className="max-h-[60vh] overflow-y-auto p-2">
              {results.isEmpty && (
                <Command.Empty className="px-4 py-8 text-center text-sm text-muted-foreground">
                  {t('globalSearch.noResults')}
                </Command.Empty>
              )}
              <GlobalSearchCommandGroups
                results={results}
                query={query}
                select={select}
                selectRegisteredCommand={selectRegisteredCommand}
              />
              <GlobalSearchResourceGroups results={results} select={select} />
              {data.failedPluginSources > 0 && (
                <output className="block px-4 py-2 text-xs text-muted-foreground">
                  {t('globalSearch.sourcesUnavailable', { ns: 'common', count: data.failedPluginSources })}
                </output>
              )}
            </Command.List>

            <div className="flex items-center gap-4 border-t border-border px-4 py-2 text-xs text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">↑↓</kbd>
                {t('globalSearch.navigate')}
              </span>
              <span className="flex items-center gap-1.5">
                <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">↵</kbd>
                {t('globalSearch.open')}
              </span>
              <span className="flex items-center gap-1.5">
                <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                  {ESC_KEY_LABEL}
                </kbd>
                {t('globalSearch.close')}
              </span>
            </div>
          </Command>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
