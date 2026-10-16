// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { TablePagination } from '@resources/components/TablePagination';
import type { PaginatedData } from '@core/api/client';

type AuditCenterPagerProps<T> = {
  data: PaginatedData<T> | undefined;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
};

export function AuditCenterPager<T>({
  data,
  page,
  pageSize,
  onPageChange,
  onPageSizeChange,
}: AuditCenterPagerProps<T>) {
  const total = data?.total ?? 0;
  const totalPages = Math.max(data?.total_pages ?? 1, 1);
  const count = data?.items.length ?? 0;
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = count === 0 ? 0 : from + count - 1;
  return (
    <TablePagination
      page={page}
      totalPages={totalPages}
      pageSize={pageSize}
      from={from}
      to={to}
      total={total}
      onPageChange={(next) => onPageChange(Math.min(Math.max(Math.trunc(next), 1), totalPages))}
      onPageSizeChange={onPageSizeChange}
    />
  );
}
