// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StorageLocationType is the union of location types the host
// manages directly. The host only ships the `local` built-in
// provider; every other location type (cloud drives, object
// stores, network shares and file-transfer protocols) is owned by
// a plugin and surfaces at runtime via
// /api/plugins/loader/storage-adapters.
// Plugins contribute their own strings into the storage adapter
// list — the host never type-checks against this list.
//
// We keep the bare `local` here so the type system can express
// the only kind the host itself can render fallback UI for.
export type StorageLocationType =
  | 'local'
  // Catch-all for plugin-owned types. The host treats these as
  // opaque strings — the storage adapter map and the
  // storageProvider() helper resolve display names and icons.
  | (string & {});

export type StorageLocation = {
  id: string;
  name: string;
  locationType: StorageLocationType;
  enabled: boolean;
  host?: string;
  port?: number;
  basePath?: string;
  region?: string;
  bucket?: string;
  endpoint?: string;
  tenantId?: string;
  siteUrl?: string;
  driveId?: string;
  shareName?: string;
  domain?: string;
  username?: string;
  authMethod?: 'password' | 'private_key' | 'password_private_key';
  tlsMode?: 'explicit' | 'implicit';
  passiveMode: boolean;
  createdAt: string;
  updatedAt: string;
};

export type StorageLocationInput = {
  name: string;
  locationType: StorageLocationType;
  enabled?: boolean;
  host?: string;
  port?: number;
  basePath?: string;
  region?: string;
  bucket?: string;
  endpoint?: string;
  tenantId?: string;
  siteUrl?: string;
  driveId?: string;
  shareName?: string;
  domain?: string;
  username?: string;
  authMethod?: 'password' | 'private_key' | 'password_private_key';
  tlsMode?: 'explicit' | 'implicit';
  passiveMode?: boolean;
  password?: string;
  privateKey?: string;
  passphrase?: string;
  caCertificate?: string;
  clientCertificate?: string;
  clientKey?: string;
  accessKeyId?: string;
  secretAccessKey?: string;
  clientId?: string;
  clientSecret?: string;
  refreshToken?: string;
  token?: string;
  // Plugin-owned fields the plugin's ConfigForm emits under its own
  // (snake_case) keys. Persisted encrypted in the per-row config
  // column; the backend mirrors well-known keys into the typed
  // columns above.
  config?: Record<string, string>;
};

export type StorageOAuthAuthorizeResponse = {
  authorizationUrl: string;
  expiresAt: string;
};

export type StorageLocationBackupMigrationResult = {
  storageLocationId: string;
  total: number;
  migrated: number;
  skipped: number;
  failed: number;
};

export type StorageLocationTestStep = {
  name: string;
  status: 'pass' | 'warn' | 'fail' | 'skip';
  detail?: string;
  latency?: string;
};

export type StorageLocationTestResult = {
  storageLocationId: string;
  locationName: string;
  locationType: string;
  overallStatus: 'pass' | 'warn' | 'fail' | 'skip';
  startedAt: string;
  completedAt: string;
  duration: string;
  steps: StorageLocationTestStep[];
};

// StorageLocationSecrets is the decrypted credential payload
// returned by GET /api/storage-locations/{id}/secrets. The dialog
// fetches this when opening an existing destination so the
// plugin's ConfigForm has the values to populate.
export type StorageLocationSecrets = {
  clientId?: string;
  clientSecret?: string;
  refreshToken?: string;
  token?: string;
  username?: string;
  password?: string;
  privateKey?: string;
  passphrase?: string;
  accessKeyId?: string;
  secretAccessKey?: string;
  // Plugin-specific fields from the per-row Config JSON map.
  extra?: Record<string, string>;
};

export type BackupEncryptionKeyResponse = {
  key: string;
  encoding: string;
  secret: string;
  secretKey: string;
  secretPath: string;
  setupShell: string;
  setupCommand: string;
};

export type BackupEncryptionKeyInstallResponse = {
  secret: string;
  secretPath: string;
  projectPath: string;
  output: string;
};

export type BackupEncryptionKeyStatus = {
  configured: boolean;
  readable: boolean;
  keyId?: string;
  path: string;
  /** McHarbor runs directly on its host; the key is a file under the data directory. */
  native?: boolean;
};

export type InstallBackupKeyInput = {
  key: string;
  // Optional Compose project path. Required when McHarbor runs
  // outside Compose (plain `docker run`, systemd, etc.)
  // so the helper container knows where to write the Docker secret.
  projectPath?: string;
};

// StorageLocationUsage mirrors modules/storage_locations.StorageUsage.
// `supported` is false when neither the host nor the owning plugin
// can report capacity; `totalBytes` is 0 when the provider has no
// hard ceiling (used figure only, no percentage).
export type StorageLocationUsage = {
  storageLocationId: string;
  supported: boolean;
  usedBytes: number;
  totalBytes: number;
  freeBytes: number;
  percent: number;
  checkedAt: string;
};
