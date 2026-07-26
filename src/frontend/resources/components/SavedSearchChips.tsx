// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBookmark, IconBookmarkFilled, IconX } from '@tabler/icons-react';
import { Button } from './ui/Button';
import { cn } from '@resources/utils/cn';
import { useSavedSearchesStore, type SavedSearch } from '@resources/stores/savedSearches';

type SavedSearchChipsProps = {
  page: string;
  currentQuery: Record<string, string | number | boolean | undefined>;
  onApply: (q: SavedSearch['query']) => void;
  className?: string;
};

function queryEquals(a: Record<string, unknown>, b: Record<string, unknown>) {
  const ak = Object.keys(a).filter((k) => a[k] !== undefined && a[k] !== '').sort();
  const bk = Object.keys(b).filter((k) => b[k] !== undefined && b[k] !== '').sort();
  if (ak.length !== bk.length) return false;
  return ak.every((k) => a[k] === b[k]);
}

export function SavedSearchChips({
  page,
  currentQuery,
  onApply,
  className,
}: SavedSearchChipsProps) {
  const { t } = useTranslation('common');
  const { forPage, remove } = useSavedSearchesStore();
  const saved = forPage(page);

  if (saved.length === 0) return null;

  const isCurrent = (q: SavedSearch['query']) => queryEquals(currentQuery as Record<string, unknown>, q);

  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      <span className="flex items-center gap-1 text-[11px] text-muted-foreground">
        <IconBookmark className="size-3" />
        {t('savedSearch.saved', { defaultValue: 'Saved' })}:
      </span>
      {saved.map((s) => {
        const active = isCurrent(s.query);
        return (
          <div
            key={s.id}
            className={cn(
              'group inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors',
              active
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border bg-card text-foreground hover:border-primary/40',
            )}
          >
            {active ? (
              <IconBookmarkFilled className="size-3" />
            ) : (
              <IconBookmark className="size-3" />
            )}
            <button
              type="button"
              onClick={() => onApply(s.query)}
              className="max-w-[180px] truncate"
              aria-label={t('savedSearch.apply', { defaultValue: `Apply search: ${s.name}` })}
            >
              {s.name}
            </button>
            <button
              type="button"
              onClick={() => remove(s.id)}
              className="opacity-50 hover:opacity-100"
              aria-label={t('savedSearch.remove', { defaultValue: `Remove saved search: ${s.name}` })}
            >
              <IconX className="size-3" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

type SaveSearchButtonProps = {
  page: string;
  name: string;
  query: Record<string, string | number | boolean | undefined>;
  className?: string;
};

export function SaveSearchButton({ page, name, query, className }: SaveSearchButtonProps) {
  const { t } = useTranslation('common');
  const { save, forPage } = useSavedSearchesStore();
  const existing = forPage(page).find((q) => q.name === name);

  const handleClick = () => {
    if (existing) return;
    save(page, name, query);
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={handleClick}
      className={className}
      aria-label={t('savedSearch.save', { defaultValue: 'Save search' })}
      disabled={!!existing}
    >
      {existing ? <IconBookmarkFilled className="size-3.5" /> : <IconBookmark className="size-3.5" />}
      <span className="ml-1.5">
        {existing
          ? t('savedSearch.saved', { defaultValue: 'Saved' })
          : t('savedSearch.save', { defaultValue: 'Save' })}
      </span>
    </Button>
  );
}
