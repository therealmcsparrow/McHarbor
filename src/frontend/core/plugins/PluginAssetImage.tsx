// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useThemeStore } from '@resources/stores/theme';
import { catalogIconURL, pluginAssetURL } from './asset-url';

type PluginAssetImageProps = {
  /** Plugin slug the icon is shipped by. */
  slug: string;
  /** Asset filename inside the plugin's icons/ dir, e.g. "plugin.svg". */
  icon: string;
  className?: string;
  /**
   * Alt text. Omit (or pass "") for a decorative mark sitting next to
   * a visible label — the image is then hidden from assistive tech.
   */
  alt?: string;
  /** The icon lives in a remote catalog rather than on this host (entry.icon_url is set). */
  fromCatalog?: boolean;
};

/**
 * A plugin's own mark, following the active theme.
 *
 * Plugins ship one brand icon (`plugin.svg`) and may ship a second one
 * for dark surfaces (`plugin_dark.svg`). Which file is served is decided
 * by the host: this only tells it which theme is on screen, and a plugin
 * that ships no dark variant keeps rendering its single icon. That keeps
 * the fallback off the network — no probe request, no 404 in the console,
 * no frame where the wrong mark is painted.
 *
 * Every surface that renders a plugin-supplied icon (store cards, adapter
 * pickers, provider tables, connection cards) goes through this so the
 * dark mark appears everywhere at once as soon as a plugin adds one.
 */
export function PluginAssetImage({ slug, icon, className, alt, fromCatalog }: PluginAssetImageProps) {
  const resolvedTheme = useThemeStore((s) => s.resolvedTheme);
  const variant = resolvedTheme === 'dark' ? 'dark' : undefined;
  const url = fromCatalog ? catalogIconURL(slug, variant) : pluginAssetURL(slug, icon, variant);
  if (!url) return null;
  return <img src={url} alt={alt ?? ''} className={className} aria-hidden={alt ? undefined : true} />;
}
