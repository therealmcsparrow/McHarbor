// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type {
  StorageLocation,
  StorageLocationInput,
  StorageLocationSecrets,
  StorageLocationType,
} from '@resources/hooks/useStorageLocations';

export const DEFAULT_FORM: StorageLocationInput = {
  name: '',
  locationType: 'local',
  enabled: true,
  passiveMode: true,
  basePath: '',
};

export function supportsConsent(locationType: StorageLocationType) {
  // Plugin-owned destinations run their own consent flow. The
  // host has no opinion about which locations need a consent step
  // — the dialog's "Save and connect" button is only enabled for
  // plugin-owned types where the plugin's SetupGuide mounts and
  // exposes the consent URL.
  return locationType !== 'local';
}

export function formFromLocation(source?: StorageLocation | null): StorageLocationInput {
  if (!source) return DEFAULT_FORM;
  return {
    name: source.name,
    locationType: source.locationType,
    enabled: source.enabled,
    host: source.host ?? '',
    port: source.port,
    basePath: source.basePath ?? '',
    region: source.region ?? '',
    bucket: source.bucket ?? '',
    endpoint: source.endpoint ?? '',
    tenantId: source.tenantId ?? '',
    siteUrl: source.siteUrl ?? '',
    driveId: source.driveId ?? '',
    shareName: source.shareName ?? '',
    domain: source.domain ?? '',
    username: source.username ?? '',
    authMethod: source.authMethod ?? (source.locationType === 'local' ? undefined : 'password'),
    tlsMode: source.tlsMode,
    passiveMode: source.passiveMode,
  };
}

export function cleanInput(input: StorageLocationInput): StorageLocationInput {
  const { config, ...rest } = input;
  const kept: Record<string, unknown> = Object.fromEntries(
    Object.entries(rest).filter(([, value]) => value !== '' && value !== undefined),
  );
  const cleaned = kept as StorageLocationInput;
  const cleanConfig = Object.fromEntries(
    Object.entries(config ?? {}).filter(([, value]) => typeof value === 'string' && value !== ''),
  );
  if (Object.keys(cleanConfig).length > 0) {
    cleaned.config = cleanConfig;
  }
  return cleaned;
}

// HOST_FORM_KEYS is the set of typed StorageLocationInput keys. A
// plugin ConfigForm writes whatever keys it likes (some plugins use
// the host's camelCase names, others snake_case). Keys the host
// recognises land on the typed form;
// everything else goes into `form.config`, which the backend stores
// encrypted and hands back to the plugin verbatim on Test / Upload /
// OAuth.
const HOST_FORM_KEYS = new Set<string>([
  'name',
  'locationType',
  'enabled',
  'host',
  'port',
  'basePath',
  'region',
  'bucket',
  'endpoint',
  'tenantId',
  'siteUrl',
  'driveId',
  'shareName',
  'domain',
  'username',
  'authMethod',
  'tlsMode',
  'passiveMode',
  'password',
  'privateKey',
  'passphrase',
  'caCertificate',
  'clientCertificate',
  'clientKey',
  'accessKeyId',
  'secretAccessKey',
  'clientId',
  'clientSecret',
  'refreshToken',
  'token',
]);

// pluginFormValue flattens the typed form plus the plugin config map
// into the single string map a plugin ConfigForm reads from.
export function pluginFormValue(form: StorageLocationInput): Record<string, string> {
  const { config, ...rest } = form;
  const flat: Record<string, unknown> = {
    ...rest,
    ...config,
  };
  return flat as Record<string, string>;
}

// pluginFormPatch splits what a plugin ConfigForm emitted back into a
// typed-form patch (host keys) and a replacement config map (the rest).
export function pluginFormPatch(next: Record<string, string>): Partial<StorageLocationInput> {
  const patch: Record<string, unknown> = {};
  const config: Record<string, string> = {};
  for (const [key, value] of Object.entries(next)) {
    if (HOST_FORM_KEYS.has(key)) {
      patch[key] = value;
    } else if (key !== 'config') {
      config[key] = value;
    }
  }
  patch.config = config;
  return patch as Partial<StorageLocationInput>;
}

// secretsToFormPatch maps the decrypted secrets payload to the
// keys the dialog's typed form recognises. The plugin's ConfigForm
// reads `form.<fieldKey>` directly, so feeding the populated
// values into the typed form is enough — no plugin-specific glue
// required.
//
// Plugin-only fields (everything in `extra`) are merged under
// their original keys so the plugin's ConfigForm sees a
// populated value too. The dialog's typed save path persists
// `clientId` / `clientSecret` / `basePath` / `refreshToken` to
// first-class columns; the plugin's other fields (folderId,
// driveId, etc.) are appended to the per-row Config JSON map
// by the backend's typed save handler.
export function secretsToFormPatch(secrets: StorageLocationSecrets): Partial<StorageLocationInput> {
  const patch: Partial<StorageLocationInput> & Record<string, string> = {};
  if (secrets.clientId) patch.clientId = secrets.clientId;
  if (secrets.clientSecret) patch.clientSecret = secrets.clientSecret;
  if (secrets.refreshToken) patch.refreshToken = secrets.refreshToken;
  if (secrets.token) patch.token = secrets.token;
  if (secrets.username) patch.username = secrets.username;
  if (secrets.password) patch.password = secrets.password;
  if (secrets.privateKey) patch.privateKey = secrets.privateKey;
  if (secrets.passphrase) patch.passphrase = secrets.passphrase;
  if (secrets.accessKeyId) patch.accessKeyId = secrets.accessKeyId;
  if (secrets.secretAccessKey) patch.secretAccessKey = secrets.secretAccessKey;
  if (secrets.extra) {
    // Plugin-owned keys come back from the encrypted per-row config
    // map. They are kept in `form.config` so the plugin's ConfigForm
    // sees them (via pluginFormValue) and the next save round-trips
    // them unchanged.
    const config: Record<string, string> = {};
    for (const [k, v] of Object.entries(secrets.extra)) {
      if (typeof v === 'string' && v !== '' && !HOST_FORM_KEYS.has(k)) {
        config[k] = v;
      }
    }
    if (Object.keys(config).length > 0) {
      patch.config = config;
    }
  }
  return patch;
}
