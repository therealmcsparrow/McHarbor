// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronLeft, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Flex } from '@resources/components/ui/Layout';
import { Select } from '@resources/components/ui/Select';
import { Text as UiText } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';

// Row counts every list in the app offers. 100 is the ceiling because the
// backend's `response.ParsePagination` clamps `per_page` at 100 — a larger
// option would be trimmed server-side and a server-paged list would then
// report a page count it cannot actually deliver.
export const tablePageSizeOptions = [25, 50, 75, 100] as const;
export const tableDefaultPageSize = tablePageSizeOptions[0];

type TablePaginationProps = {
  // 1-based, so it matches what the page-number input shows.
  page: number;
  totalPages: number;
  pageSize: number;
  from: number;
  to: number;
  total: number;
  pageSizeOptions?: readonly number[];
  onPageChange: (page: number) => void;
  // Omit to hide the rows-per-page selector — for a list whose page size
  // is fixed by something other than the reader.
  onPageSizeChange?: (pageSize: number) => void;
  // Extra content on the left, above the "showing" line. The notifications
  // list uses it for its unread count.
  leading?: ReactNode;
  className?: string;
};

// The one pager. It is purely presentational: `DataGrid` feeds it from
// TanStack's client-side table state, and server-paged lists feed it from
// their own query state. Neither concern leaks in here.
export function TablePagination({
  page,
  totalPages,
  pageSize,
  from,
  to,
  total,
  pageSizeOptions = tablePageSizeOptions,
  onPageChange,
  onPageSizeChange,
  leading,
  className,
}: TablePaginationProps) {
  const { t } = useTranslation('common');
  const [pageDraft, setPageDraft] = useState(String(page));

  // The page also moves without the input — prev/next, a delete emptying the
  // last page, a page-size change — so mirror it back into the draft.
  const [draftPage, setDraftPage] = useState(page);
  if (draftPage !== page) {
    setDraftPage(page);
    setPageDraft(String(page));
  }

  function commitPageDraft() {
    const parsed = Number.parseInt(pageDraft, 10);
    if (Number.isNaN(parsed)) {
      setPageDraft(String(page));
      return;
    }
    const clamped = Math.min(Math.max(parsed, 1), Math.max(totalPages, 1));
    onPageChange(clamped);
    setPageDraft(String(clamped));
  }

  // A caller may run on a size the shared list does not carry (a compact tab
  // at 10 rows). Fold it in rather than rendering a select with nothing
  // selected, so the reader can still see — and leave — where they are.
  const options = pageSizeOptions.includes(pageSize)
    ? [...pageSizeOptions]
    : [...pageSizeOptions, pageSize].sort((a, b) => a - b);

  // A size choice only means something once there is more data than the
  // smallest option holds; the arrows only once there are two pages.
  const smallestPageSize = Math.min(...options);
  const showPageSize = Boolean(onPageSizeChange) && options.length > 1 && total > smallestPageSize;
  const showPager = totalPages > 1;

  if (!showPager && !showPageSize && !leading) {
    return null;
  }

  return (
    <Flex align="center" justify="between" wrap gap={3} className={cn('shrink-0', className)}>
      <Flex direction="col" gap={1}>
        {leading}
        <UiText size="xs" tone="muted">
          {t('dataGrid.showing', { from, to, total })}
        </UiText>
      </Flex>

      <Flex align="center" wrap gap={4}>
        {showPageSize && (
          <Flex align="center" gap={2}>
            <UiText size="xs" tone="muted" as="span">
              {t('dataGrid.rowsPerPage')}
            </UiText>
            <Select
              variant="outline"
              className="w-20 text-xs"
              ariaLabel={t('dataGrid.rowsPerPage')}
              value={String(pageSize)}
              onChange={(value) => onPageSizeChange?.(Number(value))}
              options={options.map((option) => ({
                value: String(option),
                label: String(option),
              }))}
            />
          </Flex>
        )}

        {showPager && (
          <Flex align="center" gap={2}>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="text-xs"
              disabled={page <= 1}
              onClick={() => onPageChange(page - 1)}
            >
              <IconChevronLeft className="h-3.5 w-3.5" />
              {t('dataGrid.prev')}
            </Button>
            <Input
              variant="outline"
              type="number"
              min={1}
              max={totalPages}
              className="w-16 py-0 text-center text-xs"
              aria-label={t('dataGrid.goToPage')}
              title={t('dataGrid.goToPage')}
              value={pageDraft}
              onChange={(evt) => setPageDraft(evt.target.value)}
              onBlur={commitPageDraft}
              onKeyDown={(evt) => {
                if (evt.key === 'Enter') {
                  evt.preventDefault();
                  commitPageDraft();
                }
              }}
            />
            <UiText size="xs" tone="muted" as="span">
              {t('dataGrid.pageOf', { total: totalPages })}
            </UiText>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="text-xs"
              disabled={page >= totalPages}
              onClick={() => onPageChange(page + 1)}
            >
              {t('dataGrid.next')}
              <IconChevronRight className="h-3.5 w-3.5" />
            </Button>
          </Flex>
        )}
      </Flex>
    </Flex>
  );
}
