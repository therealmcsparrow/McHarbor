// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type AppSettings = {
  appName: string;
  sessionTimeout: number;
  autoRefreshInterval: number;
  enableRegistration: boolean;
  defaultEnvironment: string;
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

export type CertInfo = {
  subject: string;
  issuer: string;
  notBefore: string;
  notAfter: string;
  serialNumber: string;
  dnsNames: string[];
};

export type TLSStatus = {
  enabled: boolean;
  forceHttps: boolean;
  hasCert: boolean;
  certInfo?: CertInfo;
};

export type AgentSettingsData = {
  eventMode: string;
  eventPollInterval: number;
  pingInterval: number;
  metricsEnabled: boolean;
  requestTimeout: number;
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
  backupRetentionCount: number;
  backupRetentionDays: number;
};
