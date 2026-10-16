// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import type { Secret } from '@core/types/secrets';
import { useEnvironmentStore } from '@resources/stores/environment';

export const SECRETS_QUERY_KEY = 'secrets';

/**
 * Lists secret metadata. Values are never in this payload — the Go response
 * type has no value field at all — so this is safe to cache like any other
 * list query.
 */
export function useSecretsList(options: { includeSystem?: boolean } = {}) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const includeSystem = options.includeSystem ?? false;

  return useQuery({
    queryKey: [SECRETS_QUERY_KEY, envId, includeSystem],
    queryFn: () =>
      api
        .get<PaginatedData<Secret>>('/secrets', {
          ...(envId ? { env: envId } : {}),
          ...(includeSystem ? { includeSystem: 'true' } : {}),
          per_page: '100',
        })
        .then((r) => r.data?.items ?? []),
  });
}
