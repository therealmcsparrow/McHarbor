// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ColumnDef, RowData } from '@resources/components/table-features';
import {
  TABLE_COLUMN_WIDTHS,
  type TableColumnAlign,
  type TableColumnMeta,
  type TableColumnWidth,
  type TableDensity,
} from './types';

export type LayoutColumn = {
  id: string;
  visible: boolean;
  size: TableColumnWidth;
  headerAlign?: TableColumnAlign;
  cellAlign?: TableColumnAlign;
  /** Stored only when it differs from the column's own default. */
  sortable?: boolean;
};

export type LayoutPayload = {
  v: 1;
  columns: LayoutColumn[];
  density: TableDensity;
  pageSize: number;
  autoRefreshMs: number;
};

export type DeclaredColumn = {
  id: string;
  locked: boolean;
  defaultVisible: boolean;
  width: TableColumnWidth;
  /** The column holds a value it can be ordered by; an actions or display-only column never does. */
  canSort: boolean;
  defaultSortable: boolean;
};

export type ResolvedLayout = {
  order: string[];
  visibility: Record<string, boolean>;
  sizes: Record<string, TableColumnWidth>;
  userSized: Set<string>;
  headerAlign: Record<string, TableColumnAlign>;
  cellAlign: Record<string, TableColumnAlign>;
  sortable: Record<string, boolean>;
};

export const WIDTH_PX: Record<Exclude<TableColumnWidth, 'auto'>, number> = {
  xs: 64,
  sm: 96,
  md: 128,
  lg: 192,
  xl: 256,
  '2xl': 320,
  '3xl': 384,
};

function normalizeAlign(value: TableColumnAlign | undefined): TableColumnAlign {
  return value === 'left' || value === 'center' || value === 'right' ? value : 'auto';
}

// An actions column renders buttons, so ordering rows by it means nothing even when it has an accessor.
const UNSORTABLE_IDS = new Set(['actions', 'select']);

function hasAccessor<T extends RowData>(column: ColumnDef<T, unknown>): boolean {
  const def = column as { accessorKey?: unknown; accessorFn?: unknown };
  return typeof def.accessorKey === 'string' || typeof def.accessorFn === 'function';
}

function columnId<T extends RowData>(column: ColumnDef<T, unknown>): string | undefined {
  if (column.id) return column.id;
  const key = (column as { accessorKey?: unknown }).accessorKey;
  return typeof key === 'string' ? key : undefined;
}

export function declaredColumns<T extends RowData>(columns: ColumnDef<T, unknown>[]): DeclaredColumn[] {
  const out: DeclaredColumn[] = [];
  for (const column of columns) {
    const id = columnId(column);
    if (!id || id === 'select') continue;
    const meta = column.meta as TableColumnMeta | undefined;
    // Every column but actions and selection can sort: a display-only column sorts by meta.sortValue or the row field.
    const canSort = !UNSORTABLE_IDS.has(id);
    out.push({
      id,
      locked: meta?.locked === true,
      defaultVisible: meta?.defaultVisible !== false,
      width: meta?.width ?? 'auto',
      canSort,
      // A column with no value of its own starts unsorted until a sortValue is declared or the operator turns it on.
      defaultSortable:
        canSort &&
        (hasAccessor(column) || typeof meta?.sortValue === 'function') &&
        (column as { enableSorting?: boolean }).enableSorting !== false,
    });
  }
  return out;
}

export function resolveLayout(declared: DeclaredColumn[], saved: LayoutPayload | null): ResolvedLayout {
  const byId = new Map(declared.map((column) => [column.id, column]));
  const visibility: Record<string, boolean> = {};
  const sizes: Record<string, TableColumnWidth> = {};
  const userSized = new Set<string>();
  const headerAlign: Record<string, TableColumnAlign> = {};
  const cellAlign: Record<string, TableColumnAlign> = {};
  const sortable: Record<string, boolean> = {};
  for (const column of declared) {
    sortable[column.id] = column.defaultSortable;
    visibility[column.id] = column.defaultVisible;
    sizes[column.id] = column.width;
    headerAlign[column.id] = 'auto';
    cellAlign[column.id] = 'auto';
  }

  const savedColumns = (saved?.columns ?? []).filter((column) => byId.has(column.id));
  for (const column of savedColumns) {
    if (!byId.get(column.id)?.locked) visibility[column.id] = column.visible;
    sizes[column.id] = column.size;
    if (column.size !== byId.get(column.id)?.width) userSized.add(column.id);
    headerAlign[column.id] = normalizeAlign(column.headerAlign);
    cellAlign[column.id] = normalizeAlign(column.cellAlign);
    if (typeof column.sortable === 'boolean' && byId.get(column.id)?.canSort) sortable[column.id] = column.sortable;
  }

  const order = savedColumns.map((column) => column.id).filter((id) => !byId.get(id)?.locked);
  const placed = new Set(order);
  declared.forEach((column, index) => {
    if (column.locked || placed.has(column.id)) return;
    const previous = declared
      .slice(0, index)
      .reverse()
      .find((candidate) => placed.has(candidate.id));
    const at = previous ? order.indexOf(previous.id) + 1 : 0;
    order.splice(at, 0, column.id);
    placed.add(column.id);
  });

  declared.forEach((column, index) => {
    if (!column.locked) return;
    order.splice(Math.min(index, order.length), 0, column.id);
  });

  return { order, visibility, sizes, userSized, headerAlign, cellAlign, sortable };
}

export function toLayoutColumns(declared: DeclaredColumn[], resolved: ResolvedLayout): LayoutColumn[] {
  const known = new Set(declared.map((column) => column.id));
  const byId = new Map(declared.map((column) => [column.id, column]));
  return resolved.order
    .filter((id) => known.has(id))
    .map((id) => {
      const column: LayoutColumn = {
        id,
        visible: resolved.visibility[id] !== false,
        size: resolved.sizes[id] ?? 'auto',
      };
      const headerAlign = resolved.headerAlign[id] ?? 'auto';
      const cellAlign = resolved.cellAlign[id] ?? 'auto';
      if (headerAlign !== 'auto') column.headerAlign = headerAlign;
      if (cellAlign !== 'auto') column.cellAlign = cellAlign;
      const declaredColumn = byId.get(id);
      const sortable = resolved.sortable?.[id];
      if (declaredColumn?.canSort && typeof sortable === 'boolean' && sortable !== declaredColumn.defaultSortable) {
        column.sortable = sortable;
      }
      return column;
    });
}

export function nearestWidth(px: number): TableColumnWidth {
  let best: TableColumnWidth = TABLE_COLUMN_WIDTHS[0] ?? 'xs';
  let bestDistance = Number.POSITIVE_INFINITY;
  for (const width of TABLE_COLUMN_WIDTHS) {
    if (width === 'auto') continue;
    const distance = Math.abs(WIDTH_PX[width] - px);
    if (distance < bestDistance) {
      best = width;
      bestDistance = distance;
    }
  }
  return best;
}

export function stableStringify(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
      .filter(([, entry]) => entry !== undefined)
      .sort(([a], [b]) => a.localeCompare(b));
    return `{${entries.map(([key, entry]) => `${JSON.stringify(key)}:${stableStringify(entry)}`).join(',')}}`;
  }
  return JSON.stringify(value);
}
