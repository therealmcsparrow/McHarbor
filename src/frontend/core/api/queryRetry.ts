// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { isClientError } from '@resources/utils/api-mutation';

/** shouldRetryQuery retries a failed query once, and never a 4xx, which the identical request would only repeat. */
export function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  return failureCount < 1 && !isClientError(error);
}
