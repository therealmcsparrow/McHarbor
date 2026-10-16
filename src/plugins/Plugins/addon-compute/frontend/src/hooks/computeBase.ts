// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ApiResponse } from '@core/api/client';
import { apiErrorOf, describeApiError, ApiError, type ErrorDetails } from '@resources/utils/api-mutation';

/**
 * A failed request, with the backend's i18n code kept intact.
 *
 * The plugin's envelope carries its reason as `code` (`err.compute.agent_failed`
 * and friends) and leaves `error` empty, so the host's `assertSuccess` — which
 * reads `error` — would turn every failure into "Request failed". Carrying the
 * code lets the UI say the actual sentence, which for a capability probe is the
 * whole point of the screen.
 */
export class ComputeRequestError extends ApiError {
  constructor(
    readonly code?: string,
    details?: Omit<ErrorDetails, 'at'>,
  ) {
    const summary = details?.summary ?? describeApiError({ code });
    super(summary, details?.status, { summary, ...details, code });
    this.name = 'ComputeRequestError';
  }
}

/** Unwrap an envelope, throwing a `ComputeRequestError` on failure. */
export function unwrap<T>(res: ApiResponse<T>): T {
  if (!res.success) throw new ComputeRequestError(res.code ?? res.error, apiErrorOf(res).details);
  return res.data as T;
}

export const KEYS = {
  overview: ['compute', 'overview'] as const,
  environments: ['compute', 'environments'] as const,
  capability: (envId: string) => ['compute', 'capability', envId] as const,
  pools: (envId?: string) => ['compute', 'pools', envId ?? 'all'] as const,
  poolBackends: (envId: string) => ['compute', 'pool-backends', envId] as const,
  networks: (envId?: string) => ['compute', 'networks', envId ?? 'all'] as const,
  links: (envId: string) => ['compute', 'links', envId] as const,
  leases: (networkId: string) => ['compute', 'leases', networkId] as const,
  images: (envId?: string, kind?: string) => ['compute', 'images', envId ?? 'all', kind ?? 'all'] as const,
  imageLibrary: ['compute', 'image-library'] as const,
};

/** Only send a query string when there is something to put in it. */
export function params(entries: Record<string, string | undefined>): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(entries)) {
    if (value) out[key] = value;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}
