// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Row, RowData } from '@resources/components/table-features';

export function bucketRows<T extends RowData>(
  rows: Row<T>[],
  key: (row: T) => string,
): { key: string; rows: Row<T>[] }[] {
  const order: string[] = [];
  const byKey = new Map<string, Row<T>[]>();
  for (const row of rows) {
    const bucket = key(row.original);
    const existing = byKey.get(bucket);
    if (existing) {
      existing.push(row);
      continue;
    }
    order.push(bucket);
    byKey.set(bucket, [row]);
  }
  return order.map((bucket) => ({ key: bucket, rows: byKey.get(bucket) ?? [] }));
}
