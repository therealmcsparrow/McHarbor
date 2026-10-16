// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { QueryClient } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { server } from '../../test/server';
import { shouldRetryQuery } from './queryRetry';

async function attempts(status: number): Promise<number> {
  let calls = 0;
  server.use(
    http.get('*/api/retry-probe', () => {
      calls += 1;
      return HttpResponse.json({ success: false, error: 'nope' }, { status });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: shouldRetryQuery, retryDelay: 0 } } });
  await client
    .fetchQuery({ queryKey: ['retry-probe', status], queryFn: () => api.get('/retry-probe').then(assertSuccess) })
    .catch(() => undefined);
  return calls;
}

describe('shouldRetryQuery', () => {
  it.each([400, 401, 403, 404, 409])('does not retry a %i', async (status) => {
    expect(await attempts(status)).toBe(1);
  });

  it.each([500, 502, 503])('retries a %i exactly once', async (status) => {
    expect(await attempts(status)).toBe(2);
  });

  it('retries an error that carries no status, such as a network failure', () => {
    expect(shouldRetryQuery(0, new TypeError('Failed to fetch'))).toBe(true);
    expect(shouldRetryQuery(1, new TypeError('Failed to fetch'))).toBe(false);
  });
});
