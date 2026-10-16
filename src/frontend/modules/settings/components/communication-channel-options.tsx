// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { IconPuzzle } from '@tabler/icons-react';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { NotificationAdapterInfo } from '../hooks/useCommunicationAdapters';
import type { ChannelType } from '../hooks/useNotificationChannels';

/** Union of every selectable type in the "Add Communication" picker. */
export type CommunicationCreateType = ChannelType;

/**
 * One tile in the "Add Communication" picker grid. Communication
 * transports are plugin-owned, so labels and icons come from adapters.
 */
export type CommunicationChannelOption = {
  type: CommunicationCreateType;
  /**
   * Plugin adapters surface their own DisplayName here so the host
   * doesn't need to mirror every plugin's label into the settings i18n bundle.
   */
  label: string;
  description: string;
  /**
   * Icon for the tile. This is an <img> element for plugin-supplied
   * SVG assets, or a fallback puzzle icon.
   */
  icon: ReactNode;
  /** True when `label` is a t() key; false when it's a literal. */
  labelIsKey: boolean;
  /** True when `description` is a t() key; false when it's a literal. */
  descriptionIsKey: boolean;
};

/**
 * Resolves a plugin adapter's Icon (asset filename declared in
 * RegisterAdapters) to a sized <img> ReactNode via
 * /api/plugins/loader/icon. Returns IconPuzzle as a fallback when the
 * plugin didn't ship an icon, or when the Slug is missing (legacy
 * adapter predating the Slug field).
 */
export function pluginIconNode(spec: NotificationAdapterInfo): ReactNode {
  if (spec.Slug && spec.Icon) {
    return (
      <PluginAssetImage slug={spec.Slug} icon={spec.Icon} alt={spec.DisplayName} className="size-full object-contain" />
    );
  }
  return <IconPuzzle aria-hidden="true" />;
}

/**
 * Returns one tile per plugin-contributed adapter. The host no longer
 * ships built-in communication channel implementations.
 */
export function communicationChannelOptionsWithPlugins(
  pluginAdapters: Record<string, NotificationAdapterInfo> | null | undefined,
): CommunicationChannelOption[] {
  const plugins = Object.values(pluginAdapters ?? {}).map<CommunicationChannelOption>((a) => ({
    type: a.ChannelType as CommunicationCreateType,
    label: a.DisplayName,
    description: a.Description ?? '',
    icon: pluginIconNode(a),
    labelIsKey: false,
    descriptionIsKey: false,
  }));
  return plugins;
}
