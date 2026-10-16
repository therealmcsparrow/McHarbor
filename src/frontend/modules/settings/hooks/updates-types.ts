// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type VersionCheck = {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  releaseUrl?: string;
  publishedAt?: string;
  releaseNotes?: string;
};

export type SelfUpdateState = VersionCheck & {
  lastCheckedAt?: string;
  lastError?: string;
  nextCheckAt?: string;
  intervalHours: number;
  notifiedVersion?: string;
  autoInstallVersion?: string;
  autoInstallAt?: string;
  autoInstallError?: string;
  latestAgentVersion?: string;
  agentsUpdatedAt?: string;
  nextInstallAt?: string;
  installDeferred?: 'maintenance' | 'backup_running' | 'export_running';
  installSkipped?: boolean;
};

export type SelfUpdateSettings = {
  intervalHours: number;
  channelIds: string[];
  lastSeenVersion: string;
  enabled: boolean;
  autoInstall: boolean;
  autoUpdateAgents: boolean;
  agentExcludedEnvIds: string[];
  checkCron: string;
  installCron: string;
  installWindowMinutes: number;
};

/**
 * POST /updates/agents/update starts the sweep and returns at once; per-agent progress is read
 * from the agent inventory as it moves. Must match the handler in modules/updates.
 */
export type AgentUpdateResponse = {
  message: string;
  code: string;
  queued: number;
  targetVersion: string;
};

export type NotificationChannel = {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
};

export type UpdatePolicy = {
  id: string;
  name: string;
  containerMatch: string;
  imageMatch: string;
  schedule: string;
  strategy: string;
  autoRestart: boolean;
  enabled: boolean;
  lastRunAt: string;
  lastRunStatus: string;
  createdAt: string;
  updatedAt: string;
};

export type CreatePolicyInput = {
  name: string;
  containerMatch: string;
  imageMatch: string;
  schedule: string;
  strategy: string;
  autoRestart: boolean;
};

export type UpdatePolicyInput = {
  name?: string;
  containerMatch?: string;
  imageMatch?: string;
  schedule?: string;
  strategy?: string;
  autoRestart?: boolean;
  enabled?: boolean;
};

export type FactoryResetResponse = {
  message: string;
  code: string;
  result: {
    containerId: string;
    containerName: string;
    scheduledAt: string;
  };
};

export type SelfUpdateApplyResponse = {
  message: string;
  code: string;
  result: {
    containerId: string;
    containerName: string;
    targetImage: string;
    output: string;
  };
};
