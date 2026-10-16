// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { REVEAL_QUERY_OPTIONS } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { ContainerInspect } from '@core/types/docker';

/**
 * The ordinary inspect masks credential-looking env values. This is the opt-in half:
 * `?reveal=true` needs containers.manage and writes an audit row on every call, so it
 * only runs while the operator has asked to see the values, and the plaintext is never
 * kept in the Query cache once the last subscriber unmounts.
 */
export function useContainerEnvRevealed(id: string, enabled: boolean) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-env-revealed', envId, id],
    queryFn: () =>
      api
        .get<ContainerInspect>(`/containers/${id}`, {
          ...(envId ? { env: envId } : {}),
          reveal: 'true',
        })
        .then(assertSuccess)
        .then((container) => container.Config?.Env ?? []),
    enabled: !!id && enabled,
    ...REVEAL_QUERY_OPTIONS,
  });
}
