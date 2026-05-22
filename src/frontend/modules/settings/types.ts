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

export type RetentionSettingsData = {
  auditRetentionDays: number;
  activityRetentionDays: number;
};
