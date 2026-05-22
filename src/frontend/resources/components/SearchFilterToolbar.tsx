// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBookmarkPlus, IconTrash, IconAlertCircle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { SearchMode } from '@resources/utils/search-filter';

type SavedFilterOption = {
  value: string;
  label: string;
};

type SearchFilterToolbarProps = {
  query: string;
  onQueryChange: (value: string) => void;
  mode: SearchMode;
  onModeChange: (value: SearchMode) => void;
  placeholder: string;
  regexError?: boolean;
  savedFilters?: SavedFilterOption[];
  selectedSavedFilterId?: string;
  onSavedFilterSelect?: (value: string) => void;
  onSaveFilter?: () => void;
  onDeleteSavedFilter?: () => void;
  extraControls?: ReactNode;
  className?: string;
};

export function SearchFilterToolbar({
  query,
  onQueryChange,
  mode,
  onModeChange,
  placeholder,
  regexError = false,
  savedFilters = [],
  selectedSavedFilterId = '',
  onSavedFilterSelect,
  onSaveFilter,
  onDeleteSavedFilter,
  extraControls,
  className,
}: SearchFilterToolbarProps) {
  const { t } = useTranslation('common');

  return (
    <div className={cn('rounded-lg border border-border bg-card p-3', className)}>
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center">
        <div className="flex-1">
          <Input
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder={placeholder}
            variant="outline"
          />
          {regexError && (
            <div className="mt-2 flex items-center gap-2 text-xs text-destructive">
              <IconAlertCircle className="size-3.5" />
              {t('filters.invalidRegex')}
            </div>
          )}
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <Select
            value={mode}
            onChange={(value) => onModeChange(value as SearchMode)}
            options={[
              { value: 'contains', label: t('filters.modeContains') },
              { value: 'exact', label: t('filters.modeExact') },
              { value: 'regex', label: t('filters.modeRegex') },
            ]}
            searchable={false}
            variant="outline"
            className="min-w-36"
          />

          {savedFilters.length > 0 && onSavedFilterSelect && (
            <Select
              value={selectedSavedFilterId}
              onChange={onSavedFilterSelect}
              options={[{ value: '', label: t('filters.savedFilters') }, ...savedFilters]}
              variant="outline"
              className="min-w-44"
            />
          )}

          <div className="flex items-center gap-2">
            {onSaveFilter && (
              <Button variant="outline" size="sm" onClick={onSaveFilter}>
                <IconBookmarkPlus className="size-4" />
                {t('filters.saveFilter')}
              </Button>
            )}
            {onDeleteSavedFilter && (
              <Button variant="outline" size="sm" onClick={onDeleteSavedFilter} disabled={!selectedSavedFilterId}>
                <IconTrash className="size-4" />
                {t('filters.deleteFilter')}
              </Button>
            )}
          </div>
        </div>
      </div>

      {extraControls && (
        <div className="mt-3 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          {extraControls}
        </div>
      )}
    </div>
  );
}
