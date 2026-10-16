// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Category tabs of the plugin store and the version comparison the
// Updates tab relies on. Pure data + helpers, no React.

// ponytail: ponytail ceiling — handles MAJOR.MINOR.PATCH only. Upgrade to
// real semver (Masterminds/semver) when a plugin ships a pre-release.
export const isNewerVersion = (a: string, b: string): boolean => {
  const [a1, a2, a3] = a.split('.').map(Number);
  const [b1, b2, b3] = b.split('.').map(Number);
  const diff = (a1 ?? 0) - (b1 ?? 0) || (a2 ?? 0) - (b2 ?? 0) || (a3 ?? 0) - (b3 ?? 0);
  return diff > 0;
};

export const isNewerArtifact = (
  catalogVersion: string,
  catalogBuild: string | undefined,
  installedVersion: string,
  installedBuild: string | undefined,
): boolean => {
  if (isNewerVersion(catalogVersion, installedVersion)) return true;
  if (catalogVersion !== installedVersion) return false;
  if (!catalogBuild) return false;
  if (!installedBuild) return true;
  return catalogBuild > installedBuild;
};

export type CategoryKey =
  | 'updates'
  | 'all'
  | 'addons'
  | 'ai'
  | 'communication'
  | 'identity'
  | 'logging'
  | 'scanners'
  | 'storage'
  | 'snapshots'
  | 'themes';

export type StoreCategory = { key: CategoryKey; prefix: string | null };

export const ALL_CATEGORY = { key: 'all', prefix: null } as const;

export const CATEGORIES: StoreCategory[] = [
  { key: 'updates', prefix: null },
  ALL_CATEGORY,
  { key: 'addons', prefix: null },
  { key: 'ai', prefix: 'ai-provider-' },
  { key: 'communication', prefix: 'comm-' },
  { key: 'identity', prefix: 'identity-' },
  { key: 'logging', prefix: 'logging-' },
  { key: 'storage', prefix: 'storage-' },
  { key: 'snapshots', prefix: 'snapshot-' },
  { key: 'scanners', prefix: 'scanner-' },
  { key: 'themes', prefix: 'theme-' },
];

// English fallback labels for each category tab. Used as the defaultValue
// in t(...) so the sidebar never renders a raw key like "tabs.ai" if the
// plugins namespace fails to load (e.g. stale Docker layer, missing
// bundle, hot-reload race). Each key matches the en/plugins.json
// translation, so users see the right text in both the fallback and
// the fully-translated paths.
export const CATEGORY_FALLBACK_LABELS: Record<CategoryKey, string> = {
  updates: 'Updates',
  all: 'All',
  addons: 'Add-ons',
  ai: 'AI providers',
  communication: 'Communication providers',
  identity: 'Identity providers',
  logging: 'Audit log forwarders',
  storage: 'Storage providers',
  snapshots: 'Snapshot providers',
  scanners: 'Vulnerability scanners',
  themes: 'Themes',
};

// The "addons" tab is the catch-all bucket. Build the exclude list from
// the OTHER categories so dedicated Docker apps (Nextcloud, Portainer,
// WireGuard, etc.) — none of which use a category prefix — don't flood it.
export const ADDONS_EXCLUDE_PREFIXES = CATEGORIES.filter((c) => c.prefix !== null).map((c) => c.prefix as string);
