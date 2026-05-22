// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEnvironmentStore } from '@resources/stores/environment';

/**
 * Builds an API URL with the current environment ID appended as query param.
 */
export function envUrl(
  path: string,
  params?: Record<string, string>
): string {
  const envId = useEnvironmentStore.getState().currentId;
  const searchParams = new URLSearchParams(params);
  if (envId) {
    searchParams.set('env', envId);
  }
  const qs = searchParams.toString();
  return qs ? `${path}?${qs}` : path;
}

/**
 * Returns env query params as an object for use with api.get().
 */
export function envParams(
  extra?: Record<string, string>
): Record<string, string> {
  const envId = useEnvironmentStore.getState().currentId;
  const params: Record<string, string> = { ...extra };
  if (envId) {
    params.env = envId;
  }
  return params;
}
