// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Tabler-icon registry for the plugin store catalog cards. The
// catalog JSON carries a Tabler icon *name* (the same string the
// host's ProviderCard / ChannelCard /
// storage-location-options maps use verbatim), and this map resolves
// it to the imported component at render time.
//
// Source-of-truth provenance (so the icons stay in sync with the host
// cards):
//   - Storage:       src/frontend/modules/settings/components/storage-location-options.ts
//   - Identity:      src/frontend/modules/security/components/ProviderCard.tsx
//   - Communications: src/frontend/modules/settings/components/ChannelCard.tsx
//   - AI providers:  src/frontend/modules/settings/components/AIProviderConnectionCard.tsx
//
// When a catalog JSON names an icon that isn't here, the resolver
// returns IconPuzzle so a stale or third-party catalog never crashes
// the card.
//
// Keeping this registry complete is not cosmetic. The dev catalog
// (mnt/plugin-catalog) names `plugin.svg` and ships an icons/ tree,
// so it takes the <img> branch below. The *shipped* catalog
// (src/plugins/catalog, copied to /ai-plugin-catalog by
// docker/Dockerfile) deliberately keeps Tabler names and ships **no**
// icons/ tree beside them — so in a production image this REGISTRY is
// the only branch that can resolve at all. A name missing here is a
// puzzle piece on a real store card, not a dev-only annoyance.

import {
  IconBolt,
  IconBox,
  IconBrandAws,
  IconBrandDiscord,
  IconBrandDropbox,
  IconBrandGithub,
  IconBrandGmail,
  IconBrandGoogle,
  IconBrandGoogleDrive,
  IconBrandOffice,
  IconBrandOnedrive,
  IconBrandSlack,
  IconBrandTelegram,
  IconBrandTeams,
  IconBrandWhatsapp,
  IconBrandWindows,
  IconBrain,
  IconAddressBook,
  IconCloud,
  IconCloudLock,
  IconDatabase,
  IconFolder,
  IconFolderShare,
  IconHexagons,
  IconKey,
  IconMail,
  IconMessageCircle,
  IconPuzzle,
  IconReportAnalytics,
  IconRobot,
  IconSend2,
  IconServer,
  IconServer2,
  IconServerBolt,
  IconStack2,
  IconShieldLock,
  IconShieldSearch,
  IconSparkles,
  IconWind,
  IconBell,
  IconBrandAzure,
  IconPencil,
} from '@tabler/icons-react';
import { createElement } from 'react';
import type { TablerIcon } from '@tabler/icons-react';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';

const REGISTRY: Record<string, TablerIcon> = {
  // Storage
  IconBrandAws: IconBrandAws,
  IconBrandGoogleDrive: IconBrandGoogleDrive,
  IconBrandDropbox: IconBrandDropbox,
  IconBrandOffice: IconBrandOffice,
  IconBrandOnedrive: IconBrandOnedrive,
  IconCloud: IconCloud,
  IconFolder: IconFolder,
  IconFolderShare: IconFolderShare,
  IconServer: IconServer,

  // Identity
  IconBrandAzure: IconBrandAzure,
  IconBrandGoogle: IconBrandGoogle,
  IconShieldLock: IconShieldLock,
  IconAddressBook: IconAddressBook,
  IconKey: IconKey,
  // Scanner plugins
  IconShieldSearch: IconShieldSearch,

  // Communications
  IconBrandSlack: IconBrandSlack,
  IconBrandDiscord: IconBrandDiscord,
  IconBrandTeams: IconBrandTeams,
  IconBrandTelegram: IconBrandTelegram,
  IconBrandWhatsapp: IconBrandWhatsapp,
  IconBell: IconBell,
  IconSend2: IconSend2,
  IconMessageCircle: IconMessageCircle,

  // Email
  IconMail: IconMail,
  IconBrandGmail: IconBrandGmail,
  IconBrandWindows: IconBrandWindows,

  // Logging forwarder plugins
  IconCloudLock: IconCloudLock,
  IconServerBolt: IconServerBolt,

  // AI providers
  IconSparkles: IconSparkles,
  IconBrain: IconBrain,
  IconRobot: IconRobot,
  IconWind: IconWind,
  IconBolt: IconBolt,

  // Snapshot plugins
  IconServer2: IconServer2,
  IconDatabase: IconDatabase,
  IconStack2: IconStack2,

  // Add-on plugins
  IconHexagons: IconHexagons,
  IconReportAnalytics: IconReportAnalytics,

  // Catch-all / generic
  IconPuzzle: IconPuzzle,
  IconBox: IconBox,
  IconBrandGithub: IconBrandGithub,
  IconPencil: IconPencil,
};

/**
 * Resolve a Tabler icon name from the catalog to its component.
 * Unknown names fall back to IconPuzzle so a stale catalog never
 * breaks the card render path.
 */
export function resolvePluginIcon(nameValue: string | undefined): TablerIcon {
  if (!nameValue) return IconPuzzle;
  return REGISTRY[nameValue] ?? IconPuzzle;
}

// isPluginAssetIcon reports whether `name` looks like a filename
// for an SVG/PNG asset shipped inside the plugin's icons/ dir
// (resolved via /api/plugins/loader/icon). Catalog entries can
// declare either a Tabler icon name (looked up in REGISTRY) or
// a relative asset filename ending in .svg/.png — the render
// branches on this.
function isPluginAssetIcon(itemName: string | undefined): boolean {
  if (!itemName) return false;
  return /\.(svg|png)$/i.test(itemName);
}

interface PluginIconProps {
  /**
   * Either a Tabler icon name (looked up in REGISTRY) or a
   * relative asset filename ending in .svg/.png for plugin-
   * shipped icons. Falsy falls back to IconPuzzle.
   */
  icon?: string;
  /** Required to resolve asset icons via /api/plugins/loader/icon. */
  slug?: string;
  /** Sizing/color applied to the rendered element. */
  className?: string;
  /** Alt text for `<img>` asset variants; ignored for Tabler icons. */
  alt?: string;
  /** The entry came from a remote catalog (`icon_url` set): load the mark through the host. */
  fromCatalog?: boolean;
}

// PluginIcon renders either a Tabler icon (when `icon` is a
// registered name) or an `<img>` sourced from the plugin's
// bundled asset (when `icon` ends in .svg/.png and `slug` is
// set). Catalog entries whose icon doesn't match either branch
// fall back to IconPuzzle so a stale or third-party catalog
// never crashes the card.
export function PluginIcon({ icon, slug, className, alt, fromCatalog }: PluginIconProps) {
  if (isPluginAssetIcon(icon) && slug && icon) {
    return <PluginAssetImage slug={slug} icon={icon} alt={alt} className={className} fromCatalog={fromCatalog} />;
  }
  return createElement(resolvePluginIcon(icon), { className, 'aria-hidden': 'true' });
}
