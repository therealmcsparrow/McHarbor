// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// TableFilterBar — the audit page's filter chrome, made shared.
//
// Row 1  search · match mode · saved filters
// Row 2  page filters + facets + chips   |   TableToolbar
//
// On selection, row 1 is replaced *in place* by the bulk bar rather than a
// fourth band appearing underneath — same row, same height, no layout shift.
// That is the pattern PageToolbar documented; this is where it happens.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconFilterOff, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { cn } from '@resources/utils/cn';
import type { SearchMode } from '@resources/utils/search-filter';
import { TableSavedFiltersMenu } from './TableSavedFiltersMenu';
import type { SavedFiltersController } from './useTableSavedFilters';
import type { TableSearchControl } from './types';

type TableFilterBarProps = {
  search?: TableSearchControl;
  savedFilters?: SavedFiltersController;
  onClearFilters?: () => void;
  /** Row 2, left. */
  filters?: ReactNode;
  /** Row 2, right. */
  toolbar?: ReactNode;
  /** When present and count > 0, row 1 is replaced by the bulk bar. */
  selection?: {
    count: number;
    total: number;
    selectable?: number;
    onSelectAll?: () => void;
    onClear: () => void;
    actions: ReactNode;
  };
  className?: string;
};

export function TableFilterBar({
  search,
  savedFilters,
  onClearFilters,
  filters,
  toolbar: toolbarNode,
  selection,
  className,
}: TableFilterBarProps) {
  const { t } = useTranslation('common');
  const selecting = (selection?.count ?? 0) > 0;
  const hasSecondRow = Boolean(filters || toolbarNode);

  if (!search && !savedFilters && !hasSecondRow && !selecting) {
    return null;
  }

  return (
    <div className={cn('mc-glass flex-none rounded-lg p-3', className)}>
      {selecting && selection ? (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-primary/40 bg-primary/5 px-3 py-2">
          <span className="text-sm font-medium text-foreground">
            {t('bulk.selected', { selectedCount: selection.count, totalCount: selection.total })}
          </span>
          {selection.onSelectAll && selection.count < (selection.selectable ?? selection.total) && (
            <Button variant="link" size="sm" onClick={selection.onSelectAll}>
              {t('bulk.selectAllMatching', { count: selection.selectable ?? selection.total })}
            </Button>
          )}
          <Button variant="ghost" size="icon-sm" aria-label={t('bulk.clearSelection')} onClick={selection.onClear}>
            <IconX className="size-4" />
          </Button>
          <div className="ml-auto flex flex-wrap items-center gap-2">{selection.actions}</div>
        </div>
      ) : (
        (search || savedFilters) && (
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
            {search && (
              <div className="flex-1">
                <Input
                  value={search.value}
                  onChange={(evt) => search.onChange(evt.target.value)}
                  placeholder={search.placeholder}
                  variant="outline"
                />
                {search.error && (
                  <div className="mt-2 flex items-center gap-2 text-xs text-destructive">
                    <IconAlertCircle className="size-3.5" />
                    {t('filters.invalidRegex')}
                  </div>
                )}
              </div>
            )}
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              {search && (
                <Select
                  value={search.mode}
                  onChange={(value) => search.onModeChange(value as SearchMode)}
                  options={[
                    { value: 'contains', label: t('filters.modeContains') },
                    { value: 'exact', label: t('filters.modeExact') },
                    { value: 'regex', label: t('filters.modeRegex') },
                  ]}
                  searchable={false}
                  variant="outline"
                  className="min-w-36"
                />
              )}
              {savedFilters && <TableSavedFiltersMenu controller={savedFilters} />}
              {savedFilters && onClearFilters && (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!savedFilters.canClear || savedFilters.pristine !== false}
                  onClick={onClearFilters}
                >
                  <IconFilterOff className="size-4" />
                  {t('filters.clear')}
                </Button>
              )}
            </div>
          </div>
        )
      )}

      {hasSecondRow && (
        <div
          className={cn(
            'flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between',
            (search || savedFilters || selecting) && 'mt-3',
          )}
        >
          {/* Time Range comes first when a page has one — it is the filter
              readers reach for most, so it holds the leftmost slot on every
              table page rather than landing wherever a page happened to put it.

              Controls sit in a row from `md` up. Give anything dropped in here
              an explicit width: `Select`'s trigger is `w-full` by default, and a
              full-width flex child wraps onto its own line, which reads as a
              vertical stack. `DateRangeFilter` already carries `min-w-44`. */}
          <div className="flex min-w-0 flex-col gap-3 md:flex-row md:flex-wrap md:items-center">{filters}</div>
          {toolbarNode}
        </div>
      )}
    </div>
  );
}
