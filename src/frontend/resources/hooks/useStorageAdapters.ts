// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

// Backend mirror of `plugin.StorageAdapterSpec`
// (src/backend/core/plugin/adapters.go). The frontend uses this to
// render the install-aware "Add Storage Location" type picker: the
// host's built-in tile grid (local only) stays, and any
// plugin-contributed LocationType is appended as a disabled
// "Configure in plugin" tile mirroring the identity dialog UX.
//
// Adapters register at boot, so 5-minute staleTime matches the
// sibling AI / identity adapters.

export type StorageAdapterSpec = {
  // Slug is the owning plugin's slug.
  // Populated by the plugin in RegisterAdapters so the frontend can
  // resolve Icon assets via pluginAssetURL(slug, icon) without a
  // second round-trip. Empty for legacy adapters that pre-date this
  // field — those render via IconPuzzle fallback.
  Slug?: string;
  LocationType: string;
  DisplayName: string;
  // Icon is the asset filename inside the plugin's icons/ dir.
  // Resolved via pluginAssetURL(slug, icon) at render time.
  // Empty when the plugin didn't ship an icon for this
  // LocationType — renderers fall back to the default tile.
  Icon?: string;
  Description?: string;
  Capabilities?: {
    EncryptedCredentials?: boolean;
    OAuth?: boolean;
  };
  ConfigSchema?: unknown[];
};

export const storageAdapterQueryKey = ['plugins', 'loader', 'storageAdapters'] as const;

export function useStorageAdapters() {
  return useQuery({
    queryKey: storageAdapterQueryKey,
    queryFn: async () =>
      (await api.get<Record<string, StorageAdapterSpec>>('/plugins/loader/storage-adapters')).data ?? {},
    staleTime: 5 * 60_000,
  });
}
