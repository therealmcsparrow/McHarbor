// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { api, type ApiResponse } from '@core/api/client';
import { ApiError, assertSuccess } from '@resources/utils/api-mutation';

export type ScrapeResult = { lines: number };

/**
 * useTestScrape fetches /api/metrics the way a scraper would. The route
 * answers Prometheus text rather than the JSON envelope, so it goes through
 * the client's raw download path; only a failure carries an envelope, and
 * that is read for the server's own message.
 */
export function useTestScrape() {
  return useMutation({
    mutationFn: async (): Promise<ScrapeResult> => {
      const response = await api.download('/metrics');
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as ApiResponse | null;
        if (body && body.success === false) assertSuccess(body);
        throw new ApiError(`HTTP ${response.status}`, response.status);
      }
      const text = await response.text();
      return { lines: text.split('\n').filter((line) => line.trim() !== '').length };
    },
  });
}
