// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

type AboutInstall = { version: string; native?: boolean; sandboxed?: boolean };

/**
 * Whether McHarbor runs directly on its host under systemd rather than in a container. Shares
 * the footer's ["about"] query, so asking costs no extra request. False until it has loaded, so
 * a screen shows the container wording for a moment rather than nothing.
 */
export function useIsNative(): boolean {
  return useAbout()?.native === true;
}

/**
 * Whether a native McHarbor runs as the mcharbor account (installed with --no-run-as-root), so
 * systemd lets it write only under its data directory. The default native install runs as root.
 */
export function useIsSandboxed(): boolean {
  return useAbout()?.sandboxed === true;
}

function useAbout(): AboutInstall | undefined {
  const { data } = useQuery({
    queryKey: ['about'],
    queryFn: () => api.get<AboutInstall>('/about').then(assertSuccess),
    staleTime: 60_000,
  });
  return data;
}
