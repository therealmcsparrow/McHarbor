// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

// Backend mirror of `plugin.NotificationAdapterSpec`.
// The "Add Communication" picker mirrors the storage location pattern:
// built-in tile list + plugin adapter merge. Slug + Icon route to
// /api/plugins/loader/icon for plugin-supplied SVG tiles; Description
// surfaces as the tile subtitle when the plugin ships one.

// Backend mirror of `plugin.ConfigFieldDecl`
// (src/backend/core/plugin/access.go). The Go struct uses lowercase
// JSON tags (`json:"key"`, `json:"label"`, …), so the wire shape and
// the TS type agree on lowercase keys. Outer adapter specs (e.g.
// `NotificationAdapterSpec`) deliberately don't set JSON tags, so
// their fields marshal as PascalCase — only this inner schema uses
// lowercase.
export type ConfigFieldOption = {
  value: string;
  label: string;
};

export type ConfigFieldDecl = {
  key: string;
  label: string;
  /** Key for label in the owning plugin's own i18n namespace; label is the fallback. */
  label_key?: string;
  placeholder?: string;
  help?: string;
  /** Key for help in the owning plugin's own i18n namespace; help is the fallback. */
  help_key?: string;
  type: 'text' | 'string' | 'password' | 'secret' | 'number' | 'int' | 'boolean' | 'bool' | 'select' | 'multiselect';
  required?: boolean;
  default?: string;
  min?: number;
  max?: number;
  options?: ConfigFieldOption[];
};

export type NotificationAdapterInfo = {
  ChannelType: string;
  DisplayName: string;
  /** Owning plugin slug (e.g. comm-*). Empty for host-only adapters. */
  Slug?: string;
  /** Asset filename inside the plugin's icons/ directory; empty = IconPuzzle fallback. */
  Icon?: string;
  /** Short operator-facing description rendered as the tile subtitle. */
  Description?: string;
  Capabilities: {
    EncryptedCredentials: boolean;
    SupportsRichAttachments: boolean;
  };
  ConfigSchema: ConfigFieldDecl[];
};

export const notificationAdapterQueryKey = ['communication', 'notificationAdapters'] as const;

export function useNotificationAdapters() {
  return useQuery({
    queryKey: notificationAdapterQueryKey,
    queryFn: async () =>
      (await api.get<Record<string, NotificationAdapterInfo>>('/plugins/loader/notification-adapters')).data ?? {},
    // Plugins register adapters at boot, not on install. 5 minutes
    // is enough for the dialog re-open cadence; the CommunicationsTab
    // invalidates on open, so the typical flow gets a fresh fetch.
    staleTime: 5 * 60_000,
  });
}
