// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

// Backend mirror of `plugin.IdentityAdapterSpec`
// (src/backend/core/plugin/adapters.go). The host renders the
// CreateProviderDialog tile grid by merging hardcoded built-in types
// with whatever the /plugins/loader/identity-adapters endpoint returns.
//
// Identical pattern to useAIProviderAdapters / useCommunicationAdapters
// — adapters register at boot, so 5-minute staleTime matches the
// dialog re-open cadence.

export type IdentityAdapterSpec = {
  ProviderType: string;
  DisplayName: string;
  /** Owning plugin's slug. Surfaced so the
   * frontend can resolve icon assets via /api/plugins/loader/icon
   * without a second round-trip. Empty string for legacy adapters
   * that pre-date this field; renderers fall back to the default
   * tile icon. */
  Slug?: string;
  /** Asset filename inside the plugin's icons/ directory. Resolved
   * via /api/plugins/loader/icon at render time. Empty string is
   * valid — renderers fall back to the default tile icon. */
  Icon?: string;
  Capabilities: {
    OIDCRedirect: boolean;
    SAMLRedirect: boolean;
    SupportsGroupFetch: boolean;
    SupportsMetadata: boolean;
  };
  ConfigSchema?: unknown[];
};

export const identityAdapterQueryKey = ['identity', 'identityAdapters'] as const;

export function useIdentityAdapters() {
  return useQuery({
    queryKey: identityAdapterQueryKey,
    queryFn: async () =>
      (await api.get<Record<string, IdentityAdapterSpec>>('/plugins/loader/identity-adapters')).data ?? {},
    staleTime: 5 * 60_000,
  });
}
