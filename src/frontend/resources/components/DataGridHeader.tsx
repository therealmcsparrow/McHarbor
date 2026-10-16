// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowsSort } from '@tabler/icons-react';
import { flexRender, type ColumnDef, type RowData, type Table } from '@resources/components/table-features';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Flex } from '@resources/components/ui/Layout';
import { cn } from '@resources/utils/cn';
import { TableColumnResizeHandle } from '@resources/layout/table-page/TableColumnResizeHandle';
import type {
  TableColumnAlign,
  TableColumnMeta,
  TableColumnWidth,
  TableResizeProps,
} from '@resources/layout/table-page/types';

const DEFAULT_ALIGN = 'auto';

type DataGridHeaderProps<T extends RowData> = {
  table: Table<T>;
  columns: ColumnDef<T, unknown>[];
  selectable: boolean;
  tableFixed?: boolean;
} & TableResizeProps;

// Column widths come from a fixed token set, not from `header.getSize()`.
// A runtime pixel number cannot become a Tailwind arbitrary value — those
// resolve at build time — and routing it through a CSS custom property would
// still be an inline `style`, which the React standards ban. These strings are
// static so Tailwind can see them.
const columnWidthClasses: Record<TableColumnWidth, string> = {
  auto: '',
  xs: 'w-16',
  sm: 'w-24',
  md: 'w-32',
  lg: 'w-48',
  xl: 'w-64',
  '2xl': 'w-80',
  '3xl': 'w-96',
};

export function columnWidthClass(width: TableColumnWidth | undefined): string {
  return columnWidthClasses[width ?? 'auto'];
}

const columnMinWidthClasses: Record<TableColumnWidth, string> = {
  auto: '',
  xs: 'min-w-16',
  sm: 'min-w-24',
  md: 'min-w-32',
  lg: 'min-w-48',
  xl: 'min-w-64',
  '2xl': 'min-w-80',
  '3xl': 'min-w-96',
};

export function columnMinWidthClass(width: TableColumnWidth | undefined): string {
  return columnMinWidthClasses[width ?? 'auto'];
}

const headerTextAlignClasses: Record<TableColumnAlign, string> = {
  auto: 'text-left',
  left: 'text-left',
  center: 'text-center',
  right: 'text-right',
};

const headerJustifyByAlign: Record<TableColumnAlign, 'start' | 'center' | 'end'> = {
  auto: 'start',
  left: 'start',
  center: 'center',
  right: 'end',
};

const headerChildMarginClasses: Record<TableColumnAlign, string> = {
  auto: '',
  left: '[&>*]:mx-0',
  center: '[&>*]:mx-0',
  right: '[&>*]:mx-0',
};

// DataGridHeader is the `<thead>` of the table. Extracted from DataGridTable so
// the body-and-states file can stay under the 200-line guideline.
export function DataGridHeader<T extends RowData>({
  table,
  selectable,
  columnSizes,
  onColumnResize,
  resizable,
  columnAlign,
}: DataGridHeaderProps<T>) {
  const { t } = useTranslation('common');

  return (
    <thead className="sticky top-0 z-10 bg-muted">
      {table.getHeaderGroups().map((headerGroup) => (
        <tr key={headerGroup.id} className="border-b border-border">
          {selectable && (
            <th scope="col" className="w-10 px-2 py-2.5">
              <Checkbox
                checked={
                  table.getIsAllPageRowsSelected() ? true : table.getIsSomePageRowsSelected() ? 'indeterminate' : false
                }
                onCheckedChange={(checked) => table.toggleAllPageRowsSelected(!!checked)}
                aria-label={t('dataGrid.selectAll')}
              />
            </th>
          )}
          {headerGroup.headers.map((header) => {
            const canSort = header.column.getCanSort();
            const meta = header.column.columnDef.meta as TableColumnMeta | undefined;
            const size = columnSizes?.[header.column.id] ?? meta?.width;
            const align = columnAlign?.header[header.column.id] ?? DEFAULT_ALIGN;
            const locked = meta?.locked === true;
            return (
              <th
                key={header.id}
                scope="col"
                data-column-id={header.column.id}
                className={cn(
                  'relative px-4 py-2.5 font-medium text-muted-foreground',
                  columnWidthClass(size),
                  locked && columnMinWidthClass(size),
                  headerTextAlignClasses[align],
                  canSort && 'cursor-pointer select-none hover:text-foreground',
                )}
                onClick={header.column.getToggleSortingHandler()}
              >
                <Flex
                  align="center"
                  justify={headerJustifyByAlign[align]}
                  gap={1}
                  className={headerChildMarginClasses[align]}
                >
                  {flexRender(header.column.columnDef.header, header.getContext())}
                  {canSort && <IconArrowsSort className="h-3 w-3" />}
                </Flex>
                {resizable && onColumnResize && (
                  <TableColumnResizeHandle
                    label={
                      meta?.label ??
                      (typeof header.column.columnDef.header === 'string'
                        ? header.column.columnDef.header
                        : header.column.id)
                    }
                    size={size ?? 'auto'}
                    onCommit={(next) => onColumnResize(header.column.id, next)}
                  />
                )}
              </th>
            );
          })}
        </tr>
      ))}
    </thead>
  );
}
