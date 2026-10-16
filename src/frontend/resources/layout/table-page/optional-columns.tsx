// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { ColumnDef, RowData } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatBytes, formatDate } from '@resources/utils/format';
import type { TableColumnMeta, TableColumnWidth } from './types';

export type OptionalColumnOptions = { group?: string; width?: TableColumnWidth };

const EMPTY = '—';

function meta(label: string, opts: OptionalColumnOptions | undefined): TableColumnMeta {
  return { defaultVisible: false, label, group: opts?.group, width: opts?.width ?? 'md' };
}

function DateCell({ value }: { value: string | null | undefined }) {
  const prefs = useLocaleFormat();
  return (
    <span className="text-sm whitespace-nowrap text-muted-foreground">{value ? formatDate(value, prefs) : EMPTY}</span>
  );
}

function BoolCell({ value }: { value: boolean | null | undefined }) {
  const { t } = useTranslation('common');
  if (value === null || value === undefined) return <span className="text-muted-foreground">{EMPTY}</span>;
  return <span className="text-sm text-foreground">{value ? t('labels.yes') : t('labels.no')}</span>;
}

function ListCell({ value }: { value: readonly string[] | null | undefined }) {
  if (!value || value.length === 0) return <span className="text-muted-foreground">{EMPTY}</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {value.map((item) => (
        <Badge key={item} variant="secondary">
          {item}
        </Badge>
      ))}
    </div>
  );
}

export function textColumn<T extends RowData>(
  id: string,
  label: string,
  get: (row: T) => string | number | null | undefined,
  opts?: OptionalColumnOptions,
): ColumnDef<T, unknown> {
  return {
    id,
    header: label,
    accessorFn: (row) => get(row) ?? '',
    cell: ({ row }) => {
      const value = get(row.original);
      return value === null || value === undefined || value === '' ? (
        <span className="text-muted-foreground">{EMPTY}</span>
      ) : (
        <span className="block truncate text-sm text-foreground">{String(value)}</span>
      );
    },
    meta: meta(label, opts),
  };
}

export function dateColumn<T extends RowData>(
  id: string,
  label: string,
  get: (row: T) => string | null | undefined,
  opts?: OptionalColumnOptions,
): ColumnDef<T, unknown> {
  return {
    id,
    header: label,
    accessorFn: (row) => get(row) ?? '',
    sortFn: 'datetime',
    cell: ({ row }) => <DateCell value={get(row.original)} />,
    meta: meta(label, opts),
  };
}

export function boolColumn<T extends RowData>(
  id: string,
  label: string,
  get: (row: T) => boolean | null | undefined,
  opts?: OptionalColumnOptions,
): ColumnDef<T, unknown> {
  return {
    id,
    header: label,
    accessorFn: (row) => (get(row) ? 1 : 0),
    cell: ({ row }) => <BoolCell value={get(row.original)} />,
    meta: meta(label, { width: 'sm', ...opts }),
  };
}

export function listColumn<T extends RowData>(
  id: string,
  label: string,
  get: (row: T) => readonly string[] | null | undefined,
  opts?: OptionalColumnOptions,
): ColumnDef<T, unknown> {
  return {
    id,
    header: label,
    accessorFn: (row) => (get(row) ?? []).join(', '),
    cell: ({ row }) => <ListCell value={get(row.original)} />,
    meta: meta(label, { width: 'lg', ...opts }),
  };
}

export function bytesColumn<T extends RowData>(
  id: string,
  label: string,
  get: (row: T) => number | null | undefined,
  opts?: OptionalColumnOptions,
): ColumnDef<T, unknown> {
  return {
    id,
    header: label,
    accessorFn: (row) => get(row) ?? -1,
    cell: ({ row }) => {
      const value = get(row.original);
      return (
        <span className="text-sm whitespace-nowrap text-foreground">
          {value === null || value === undefined || value < 0 ? EMPTY : formatBytes(value)}
        </span>
      );
    },
    meta: meta(label, { width: 'sm', ...opts }),
  };
}
