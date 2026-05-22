// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ApiResponse } from '@core/api/client';

export type MutationMeta = {
  success?: string | ((data: unknown, variables: unknown) => string);
  error?: string;
};

export function assertSuccess<T>(res: ApiResponse<T>): T {
  if (!res.success) {
    throw new Error(res.error ?? 'Request failed');
  }
  return res.data as T;
}
