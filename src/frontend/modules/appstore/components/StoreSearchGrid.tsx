// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { IconSearch } from '@tabler/icons-react';

type StoreSearchGridProps = {
  search: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  count: number;
  countText: string;
  emptyMessage: string;
  noResultsMessage: string;
  hasItems: boolean;
  children: ReactNode;
  actions?: ReactNode;
};

export function StoreSearchGrid({
  search,
  onSearchChange,
  searchPlaceholder,
  count,
  countText,
  emptyMessage,
  noResultsMessage,
  hasItems,
  children,
  actions,
}: StoreSearchGridProps) {
  if (!hasItems && !search) {
    return (
      <div className="flex h-full min-h-48 items-center justify-center text-sm text-muted-foreground">
        {emptyMessage}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-hidden [contain:layout_paint]">
      <div className="flex shrink-0 items-center gap-3">
        <div className="relative max-w-md flex-1">
          <IconSearch className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            placeholder={searchPlaceholder}
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            className="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
          />
        </div>
        <p className="shrink-0 text-xs text-muted-foreground">
          {countText}
        </p>
        {actions}
      </div>

      {count === 0 ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          {search ? noResultsMessage : emptyMessage}
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto pr-1">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {children}
          </div>
        </div>
      )}
    </div>
  );
}
