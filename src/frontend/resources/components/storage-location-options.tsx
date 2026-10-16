// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { IconFolder, IconPuzzle } from '@tabler/icons-react';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { StorageLocationType } from '@resources/hooks/useStorageLocations';
import type { StorageAdapterSpec } from '@resources/hooks/useStorageAdapters';

export type StorageProviderOption = {
  type: StorageLocationType;
  /**
   * Either a t() key (built-in providers) or a literal display name
   * (plugin adapters surface the adapter's own DisplayName here so
   * the host doesn't need to mirror every plugin's label into the
   * settings i18n bundle). The dialog distinguishes the two via
   * `labelIsKey`.
   */
  label: string;
  description: string;
  /**
   * Icon for the tile. Either a pre-rendered Tabler icon
   * element (`<IconFolder aria-hidden />` for built-ins) or an
   * `<img>` element for plugin-supplied SVG assets. Renderers
   * apply sizing/color via a wrapper element — the icon itself
   * has no inherent size so the same `icon` value can be reused
   * at different sizes (tile vs inline label).
   */
  icon: ReactNode;
  /** True when `label` is a t() key; false when it's a literal. */
  labelIsKey: boolean;
  /** True when `description` is a t() key; false when it's a literal. */
  descriptionIsKey: boolean;
};

// pluginIconNode renders a storage adapter's own mark.
//
// Reconstructed 2026-09-11 after an over-broad deletion removed it along with
// an adjacent unused export. The contract is the one StorageAdapterSpec
// documents on its own fields: `Slug` is "empty for legacy adapters that
// pre-date this field - those render via IconPuzzle fallback", and `Icon` is
// "empty when the plugin didn't ship an icon for this LocationType".
//
// The mark is decorative: it always sits beside a visible label, so it carries
// no alt text and is hidden from assistive tech.
function pluginIconNode(adapter: StorageAdapterSpec): ReactNode {
  if (adapter.Slug && adapter.Icon) {
    return <PluginAssetImage slug={adapter.Slug} icon={adapter.Icon} />;
  }
  return <IconPuzzle aria-hidden="true" />;
}

const DEFAULT_STORAGE_PROVIDER: StorageProviderOption = {
  type: 'local',
  label: 'storage.typeLocal',
  description: 'storage.typeLocalDescription',
  icon: <IconFolder aria-hidden="true" />,
  labelIsKey: true,
  descriptionIsKey: true,
};

// The host only retains the `local` built-in storage provider.
// Every other LocationType (cloud drives, object stores, network
// shares and file-transfer protocols) is owned by a plugin and
// surfaces via storageProviderOptionsWithPlugins below.
export const STORAGE_PROVIDER_OPTIONS: StorageProviderOption[] = [
  {
    type: 'local',
    label: 'storage.typeLocal',
    description: 'storage.typeLocalDescription',
    icon: <IconFolder aria-hidden="true" />,
    labelIsKey: true,
    descriptionIsKey: true,
  },
];

export function storageProvider(
  type: StorageLocationType,
  pluginAdapters?: Record<string, StorageAdapterSpec> | null,
): StorageProviderOption {
  const builtIn = STORAGE_PROVIDER_OPTIONS.find((option) => option.type === type);
  if (builtIn) return builtIn;
  // Plugin-owned LocationType — surface the adapter's display name
  // so the host doesn't need to mirror every plugin's label, and
  // the plugin's bundled SVG via /api/plugins/loader/icon.
  const plugin = pluginAdapters?.[type];
  if (plugin) {
    return {
      type: plugin.LocationType as StorageLocationType,
      label: plugin.DisplayName,
      description: plugin.Description ?? '',
      icon: pluginIconNode(plugin),
      labelIsKey: false,
      descriptionIsKey: false,
    };
  }
  return DEFAULT_STORAGE_PROVIDER;
}

// storageProviderOptionsWithPlugins returns the static built-in list
// plus one tile per plugin-contributed adapter. Vendor-specific
// cloud destinations appear as plugin tiles here. The plugin's own
// i18n bundle and
// DisplayName provide the label/description so the host doesn't need
// to maintain a per-type i18n key for every plugin.
export function storageProviderOptionsWithPlugins(
  pluginAdapters: Record<string, StorageAdapterSpec> | null | undefined,
): StorageProviderOption[] {
  const builtIn = STORAGE_PROVIDER_OPTIONS;
  const plugins = Object.values(pluginAdapters ?? {}).map<StorageProviderOption>((adapter) => ({
    type: adapter.LocationType as StorageLocationType,
    label: adapter.DisplayName,
    description: adapter.Description ?? '',
    icon: pluginIconNode(adapter),
    labelIsKey: false,
    descriptionIsKey: false,
  }));
  return [...builtIn, ...plugins];
}
