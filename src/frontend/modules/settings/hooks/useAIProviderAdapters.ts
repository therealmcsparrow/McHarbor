// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

// Backend mirror of `plugin.AIAdapterSpec`
// (src/backend/core/plugin/adapters.go). The frontend uses this to
// drive the install-aware "Add AI Provider" dialog: the 3-bucket
// picker only surfaces types whose plugin is installed and enabled.
//
// This is the *installed plugin* surface (separate from the host-
// owned `ai_provider_connections` rows in useAIProviderConnections).
// A connection row ties a friendly name + per-instance config
// (api_key, base_url, default_model) to a ProviderType; the runtime
// catalog tells us which ProviderTypes we can actually route chat
// to at runtime.

export type AIAdapterSpec = {
  ProviderType: string;
  Slug: string;
  // Asset filename inside the plugin's icons/ dir ("plugin.svg" for
  // every AI provider today). Resolved via pluginAssetURL(Slug, Icon)
  // at render time. Optional so a stale adapter registered before the
  // field existed still type-checks — those fall back to IconBolt.
  Icon?: string;
  DisplayName: string;
  Description?: string;
  DefaultModel?: string;
  Models?: unknown[];
  Capabilities: {
    EncryptedCredentials: boolean;
    SupportsStreaming: boolean;
    SupportsTools: boolean;
    SupportsVision: boolean;
    ListModelsDynamic: boolean;
    SupportsOAuth: boolean;
  };
  ConfigSchema?: unknown[];
};

export const aiAdapterQueryKey = ['plugins', 'loader', 'aiAdapters'] as const;

export function useAIProviderAdapters() {
  return useQuery({
    queryKey: aiAdapterQueryKey,
    queryFn: async () => (await api.get<Record<string, AIAdapterSpec>>('/plugins/loader/ai-adapters')).data ?? {},
    // Plugins register adapters at boot; 5 min stale matches the
    // cadence used by useCommunicationAdapters. The dialog also
    // invalidates on open.
    staleTime: 5 * 60_000,
  });
}
