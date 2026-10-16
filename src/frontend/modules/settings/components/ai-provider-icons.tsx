// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { IconBolt } from '@tabler/icons-react';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';

/**
 * Resolves an AI provider plugin's icon asset to a sized <img>
 * ReactNode via /api/plugins/loader/icon.
 *
 * `icon` is the asset filename the plugin declares in its
 * AIAdapterSpec — "plugin.svg" for every provider today, since a
 * plugin ships exactly one brand icon under icons/. Add-on plugins
 * are the only ones that carry more than one, and they don't
 * register AI adapters.
 *
 * Mirrors `pluginIconNode` in communication-channel-options.tsx; the
 * only difference is the fallback, which stays IconBolt so a
 * provider whose plugin shipped no icon keeps the tile the AI
 * surfaces have always used.
 *
 * Falls back when the slug is missing (catalog not loaded, or a
 * connection row whose plugin was uninstalled) or the plugin
 * declared no icon. The route resolves an installed plugin's
 * icons/ dir first and the catalog mount second, so a tile for an
 * installed-but-not-loaded plugin still renders its real brand.
 */
export function aiProviderIconNode(slug: string | undefined, icon: string | undefined, className: string): ReactNode {
  if (slug && icon) {
    return <PluginAssetImage slug={slug} icon={icon} className={`${className} object-contain`} />;
  }
  return <IconBolt className={className} aria-hidden="true" />;
}
