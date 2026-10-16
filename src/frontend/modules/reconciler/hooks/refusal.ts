// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ApiResponse } from '@core/api/client';
import { assertSuccess, describeApiError } from '@resources/utils/api-mutation';
import type { TFunction } from 'i18next';

export class ReconcilerRefusal extends Error {
  constructor(
    readonly reason: string,
    message: string,
  ) {
    super(message);
  }
}

export function refusalOf<T>(res: ApiResponse<T>): T {
  if (!res.success) {
    const data = res.data as { reason?: unknown } | undefined;
    if (typeof data?.reason === 'string') {
      throw new ReconcilerRefusal(data.reason, describeApiError(res));
    }
  }
  return assertSuccess(res);
}

export function reasonOf(error: unknown): string | undefined {
  return error instanceof ReconcilerRefusal ? error.reason : undefined;
}

/**
 * The operator-facing text for a failed reconciler call: the specific refusal
 * when the server named one, otherwise the error's own (already translated)
 * message. `t` must be bound to the `reconciler` namespace.
 */
export function refusalText(error: Error, t: TFunction<'reconciler'>): string {
  const reason = reasonOf(error);
  return reason ? t(`refusal.${reason}`) : error.message;
}
