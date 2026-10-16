// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { flexRender, type Row, type RowData } from '@resources/components/table-features';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { cn } from '@resources/utils/cn';
import type {
  TableColumnAlign,
  TableColumnAlignMap,
  TableColumnMeta,
  TableColumnWidth,
} from '@resources/layout/table-page/types';

const DEFAULT_ALIGN = 'auto';

type DataGridRowProps<T extends RowData> = {
  row: Row<T>;
  selectable: boolean;
  tableFixed?: boolean;
  onRowClick?: (row: T) => void;
  isRowDisabled?: (row: T) => boolean;
  getRowClassName?: (row: T) => string | undefined;
  getRowId?: (row: T) => string;
  /** Emitted as `data-testid` on the row, for existing test selectors. */
  getRowTestId?: (row: T) => string;
  columnAlign?: TableColumnAlignMap;
  columnSizes?: Record<string, TableColumnWidth>;
  columnUserSized?: ReadonlySet<string>;
};

// DataGridRow is a single body row inside DataGridTable. Extracted from
// DataGridTable so the table-body-states file stays under the 200-line
// guideline. Padding is the audit-table baseline; density scales the vertical
// half of it from DataGridTable.
const cellAlignClasses: Record<TableColumnAlign, string> = {
  auto: '',
  left: 'text-left [&>div]:justify-start',
  center: 'text-center [&>div]:justify-center',
  right: 'text-right [&>div]:justify-end',
};

export function DataGridRow<T extends RowData>({
  row,
  selectable,
  tableFixed,
  onRowClick,
  isRowDisabled,
  getRowClassName,
  getRowId,
  getRowTestId,
  columnAlign,
  columnSizes,
  columnUserSized,
}: DataGridRowProps<T>) {
  const { t } = useTranslation('common');

  return (
    <tr
      className={cn(
        'border-b border-border transition-colors last:border-0 hover:bg-muted/30',
        onRowClick && 'cursor-pointer',
        selectable && row.getIsSelected() && 'bg-primary/5',
        getRowClassName?.(row.original),
      )}
      tabIndex={onRowClick ? 0 : undefined}
      onClick={() => onRowClick?.(row.original)}
      onKeyDown={(evt) => {
        if (!onRowClick) return;
        if (evt.target !== evt.currentTarget) return;
        if (evt.key === 'Enter' || evt.key === ' ') {
          evt.preventDefault();
          onRowClick(row.original);
        }
      }}
      data-testid={getRowTestId?.(row.original)}
      data-selected={selectable && row.getIsSelected() ? true : undefined}
    >
      {selectable && (
        <td className="w-10 px-2 py-2.5" onClick={(e) => e.stopPropagation()}>
          <Checkbox
            checked={row.getIsSelected()}
            disabled={isRowDisabled?.(row.original)}
            onCheckedChange={(checked) => row.toggleSelected(!!checked)}
            aria-label={t('dataGrid.selectRow', {
              id: getRowId ? getRowId(row.original) : row.id,
            })}
          />
        </td>
      )}
      {row.getVisibleCells().map((cell) => {
        const meta = cell.column.columnDef.meta as TableColumnMeta | undefined;
        const align = columnAlign?.cell[cell.column.id] ?? DEFAULT_ALIGN;
        const size = columnSizes?.[cell.column.id] ?? meta?.width;
        const locked = meta?.locked === true;
        const truncated = size !== undefined && size !== 'auto' && meta?.wrap !== true && !locked;
        const clamped = truncated && columnUserSized?.has(cell.column.id) === true;
        return (
          <td
            key={cell.id}
            data-column-id={cell.column.id}
            className={cn(
              'px-4 py-2.5 text-sm text-foreground',
              tableFixed || truncated ? 'overflow-hidden' : 'break-words',
              clamped && !tableFixed && 'max-w-0',
              truncated && 'truncate',
              cellAlignClasses[align],
            )}
          >
            {flexRender(cell.column.columnDef.cell, cell.getContext())}
          </td>
        );
      })}
    </tr>
  );
}
