// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { RowData, Table } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';

// Cells are resolved through the column's accessor, never through its `cell`
// renderer — `cell` returns JSX, which has no useful string form.
function cellValue(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (Array.isArray(value)) return value.join(' ');
  if (typeof value === 'object') return JSON.stringify(value);
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value);
  return '';
}

function csvField(value: string): string {
  if (value.includes(',') || value.includes('"') || value.includes('\n') || value.includes('\r')) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

function exportableColumns<T extends RowData>(table: Table<T>) {
  return table.getVisibleLeafColumns().filter((column) => {
    const meta = column.columnDef.meta as TableColumnMeta | undefined;
    if (meta?.noExport) return false;
    // The selection column has no accessor and nothing worth exporting.
    return column.id !== 'select';
  });
}

function headerText<T extends RowData>(table: Table<T>, columnId: string): string {
  const header = table
    .getHeaderGroups()
    .flatMap((group) => group.headers)
    .find((candidate) => candidate.column.id === columnId);
  const definition = header?.column.columnDef.header;
  return typeof definition === 'string' ? definition : columnId;
}

export function tableToCsv<T extends RowData>(table: Table<T>): string {
  const columns = exportableColumns(table);
  const head = columns.map((column) => csvField(headerText(table, column.id))).join(',');
  const body = table
    .getRowModel()
    .rows.map((row) => columns.map((column) => csvField(cellValue(row.getValue(column.id)))).join(','));
  return [head, ...body].join('\r\n');
}

export function tableToJson<T extends RowData>(table: Table<T>): string {
  const columns = exportableColumns(table);
  const rows = table.getRowModel().rows.map((row) => {
    const record: Record<string, string> = {};
    for (const column of columns) {
      record[headerText(table, column.id)] = cellValue(row.getValue(column.id));
    }
    return record;
  });
  return JSON.stringify(rows, null, 2);
}

export function downloadTableExport(content: string, filename: string, format: 'csv' | 'json') {
  const type = format === 'csv' ? 'text/csv;charset=utf-8' : 'application/json;charset=utf-8';
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement('a');
  link.href = url;
  link.download = `${filename}.${format}`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
