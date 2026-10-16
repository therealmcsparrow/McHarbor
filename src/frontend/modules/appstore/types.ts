// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export interface PortMapping {
  host: number;
  container: number;
  protocol: string;
}

export interface VolumeMount {
  host: string;
  container: string;
  readOnly?: boolean;
}

export type AppNetworkMode = 'default' | 'bridge' | 'host' | 'none' | 'existing';

export interface AppNetworkSettings {
  mode: AppNetworkMode;
  name: string;
  aliases: string[];
  ipv4Address: string;
  ipv6Address: string;
  macAddress: string;
}

export interface EnvVarDef {
  key: string;
  default: string;
  description: string;
  secret?: boolean;
}

export interface LatestRelease {
  version: string;
  release_date: string;
}

export interface AppTemplate {
  id: string;
  slug: string;
  name: string;
  description: string;
  category: string;
  image: string;
  logo: string;
  website: string;
  docsUrl: string;
  ports: PortMapping[];
  volumes: VolumeMount[];
  envVars: EnvVarDef[];
  composeOverride?: string;
  minMemory?: string;
  source: string;
  version: string;
  updatedAt: string;
  /**
   * What the catalog server publishes about the newest release: the real
   * version string (never the "latest" tag) and the date it shipped. Absent
   * for a catalog that does not emit the block.
   */
  latest?: LatestRelease;
  /** Newest release tag published for this app's image in its registry. */
  registryVersion?: string;
  /** When that release was built, RFC3339. */
  registryUpdatedAt?: string;
  installed: boolean;
  stackId?: string;
  installations: AppInstallation[];
}

export interface AppInstallation {
  id: string;
  stackId: string;
  stackName: string;
  environmentId?: string;
  environmentName?: string;
  installedAt: string;
}

export interface InstallRequest {
  slug: string;
  name: string;
  environmentId?: string;
  ports?: PortMapping[];
  volumes?: VolumeMount[];
  envVars?: Record<string, string>;
  network?: AppNetworkSettings;
  /**
   * Deliver any ${secret:NAME} in envVars as a file under /run/secrets and set
   * <VAR>_FILE to its path, instead of putting the value in the container's
   * environment where `docker inspect` reads it.
   */
  secretFileMode?: boolean;
}

export interface InstallResult {
  appSlug: string;
  stackId: string;
  stackName: string;
  status: string;
}

export interface InstalledApp {
  id: string;
  catalogSlug: string;
  stackId: string;
  stackName: string;
  environmentId?: string;
  environmentName?: string;
  installedAt: string;
  stackStatus: string;
}

export interface SyncStatus {
  lastSyncedAt: string;
  status: string;
  error?: string;
  appsAdded: number;
  appsUpdated: number;
}

export interface CategoryCount {
  category: string;
  count: number;
}

export interface InstallEvent {
  step: number;
  total: number;
  message: string;
  status: 'progress' | 'done' | 'error';
  phase?: string;
  stackId?: string;
  stackName?: string;
}
