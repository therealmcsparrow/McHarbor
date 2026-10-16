// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ToastPosition } from '@core/auth/useAuth';
import type { ThemeAccent, ThemeFont } from '@resources/stores/themeCustomizer';

export type AppSettings = {
  appName: string;
  sessionTimeout: number;
  autoRefreshInterval: number;
  enableRegistration: boolean;
  defaultEnvironment: string;
  defaultLanguage?: string;
};

export type TimezoneSettingsData = {
  timezone: string;
  runtimeTimezone: string;
  restartRequired: boolean;
  recreateScheduled?: boolean;
  message?: string;
};

export type WebhookItem = {
  id: string;
  name: string;
  url: string;
  events: string;
  isActive: boolean;
  createdAt: string;
};

export type PluginItem = {
  id: string;
  name: string;
  version: string;
  description: string;
  source: string;
  enabled: boolean;
  installedAt: string;
};

export type ScheduleItem = {
  id: string;
  name: string;
  description: string;
  cron: string;
  action: string;
  target: string;
  enabled: boolean;
  lastRunAt: string;
  nextRunAt: string;
};

// The TLS shapes moved to @core/types/tls: the security surface renders them
// too, and a module must not import another module's types.
export type { CertInfo, TLSStatus } from '@core/types/tls';

export type AgentSettingsData = {
  eventMode: string;
  eventPollInterval: number;
  pingInterval: number;
  metricsEnabled: boolean;
  requestTimeout: number;
  statsPushInterval: number;
};

export type AgentInfo = {
  envId: string;
  envName: string;
  status: string;
  hostname?: string;
  os?: string;
  arch?: string;
  agentVersion?: string;
  dockerVersion?: string;
  lastSeen?: string;
};

export type DirectTransferTestRequest = {
  sourceEnvId: string;
  targetEnvId: string;
};

export type DirectTransferTestResult = {
  success: boolean;
  phase: string;
  sourceEnvId: string;
  sourceName?: string;
  sourceVersion?: string;
  sourceConnected: boolean;
  targetEnvId: string;
  targetName?: string;
  targetVersion?: string;
  targetConnected: boolean;
  targetTransferUrl?: string;
  probeUrl?: string;
  statusCode?: number;
  durationMs: number;
  error?: string;
  receiver?: DirectTransferReceiver;
  responderMarker?: string;
  diagnostic?: DirectTransferDiagnostic;
};

export type DirectTransferReceiver = {
  transferId: string;
  kind: string;
  expiresAt: string;
  tokenFingerprint: string;
  agentMarker?: string;
};

export type DirectTransferDiagnostic = {
  receiverExists: boolean;
  receiverExpired: boolean;
  receiverKind?: string;
  kindMatched: boolean;
  bearerPresent: boolean;
  tokenMatched: boolean;
  remoteAddr?: string;
  responderMarker?: string;
};

export type RetentionSettingsData = {
  auditRetentionDays: number;
  activityRetentionDays: number;
  lifecycleRetentionDays: number;
  backupLogRetentionDays: number;
  scanRetentionDays: number;
  // Per-API-key request telemetry and the tombstones left by deleted keys.
  // Events age from when the request happened, tombstones from when the key
  // was deleted; one horizon governs both. 0 = keep forever.
  apiKeyUsageRetentionDays: number;
  backupRetentionCount: number;
  backupRetentionDays: number;
  activeAuditRetentionPreset: string;
  auditRetentionPresets: AuditRetentionPreset[];
  // Metric history is tiered, so it carries two horizons: how long the raw
  // 30-second samples live, and how long the 10-minute averages they fold
  // into survive afterwards.
  metricsRawHours: number;
  metricsRollupDays: number;
  metricsDiskEnabled: boolean;
  // Whether Docker exec events (overwhelmingly healthcheck probes) are stored
  // at all. Off by default — see the warning in RetentionSettingsCard.
  recordExecEvents: boolean;
};

export type AuditRetentionPreset = {
  id: string;
  name: string;
  framework: string;
  retentionDays: number;
  description: string;
  isCustom: boolean;
  sortOrder: number;
};

/**
 * The app-wide toast defaults, mirroring `coreSettings.ToastSettings`.
 *
 * `durationMs: 0` means "never auto-dismiss", which is why the backend
 * refuses that paired with `closeButton: false` — the toast would have no
 * way to leave the screen.
 *
 * `allowUserOverride` decides whether a signed-in user may replace these
 * with their own. Turning it off does not discard what people already
 * chose; their overrides simply stop applying.
 */
export type ToastSettings = {
  position: ToastPosition;
  durationMs: number;
  maxVisible: number;
  richColors: boolean;
  closeButton: boolean;
  allowUserOverride: boolean;
};

/**
 * The app-wide appearance defaults, mirroring `coreSettings.AppearanceSettings`.
 *
 * `baseFontPx` is the root font size. It is the *base* only — every relative
 * size in the app keeps its own rem multiplier and derives from this number,
 * so moving it moves `text-xs` through `text-2xl` together.
 *
 * `allowUserOverride` decides whether a signed-in user may replace these with
 * their own. Turning it off does not discard what people already chose; their
 * overrides simply stop applying.
 */
export type AppearanceSettings = {
  themeId: string;
  accent: ThemeAccent;
  font: ThemeFont;
  baseFontPx: number;
  allowUserOverride: boolean;
};
