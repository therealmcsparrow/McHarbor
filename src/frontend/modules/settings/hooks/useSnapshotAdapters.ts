// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ConfigFieldDecl } from './useCommunicationAdapters';

// Backend mirror of `plugin.SnapshotAdapterSpec`
// (src/backend/core/plugin/adapters.go). Keyed by Driver on the wire;
// the Snapshots tab uses this map to render its plugin-aware driver
// picker so users can't pick an uninstalled plugin and so picking
// one auto-fills plugin_slug + driver on the form.

export type SnapshotAdapterInfo = {
  Driver: string;
  DisplayName: string;
  /** Owning plugin slug. Empty for host-only adapters. */
  Slug?: string;
  /** Asset filename inside the plugin's icons/ directory; empty = IconCamera fallback. */
  Icon?: string;
  /** Short operator-facing description rendered as the picker subtitle. */
  Description?: string;
  Capabilities: {
    SupportsMount: boolean;
    SupportsRemoteTarget: boolean;
    RequiresAgent: boolean;
  };
  // Null on the wire, not just empty: an adapter with no fields (the
  // built-in Windows VSS driver, or any plugin that declares none) marshals
  // its nil Go slice to JSON null. Typing it as a plain array let a call site
  // read .length straight off it and throw.
  ConfigSchema: ConfigFieldDecl[] | null;
};

export const snapshotAdapterQueryKey = ['snapshotAdapters'] as const;

export function useSnapshotAdapters() {
  return useQuery({
    queryKey: snapshotAdapterQueryKey,
    queryFn: async () =>
      (await api.get<Record<string, SnapshotAdapterInfo>>('/plugins/loader/snapshot-adapters')).data ?? {},
    // Plugins register adapters at boot. 5 minutes is enough for the
    // dialog re-open cadence; a fresh install picks up within one
    // staleTime window. Mirrors useNotificationAdapters.
    staleTime: 5 * 60_000,
  });
}
