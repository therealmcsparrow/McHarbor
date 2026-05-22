// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type { ApiResponse, PaginatedData } from '@core/api/client';

export type GridId = string;

export type SortDirection = 'asc' | 'desc';

export type ColumnConfig = {
  key: string;
  label: string;
  width?: number;
  minWidth?: number;
  sortable?: boolean;
  visible?: boolean;
  align?: 'left' | 'center' | 'right';
};

export type PaginationState = {
  page: number;
  perPage: number;
  total: number;
};
