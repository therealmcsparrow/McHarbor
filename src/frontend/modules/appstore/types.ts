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

export interface EnvVarDef {
  key: string;
  default: string;
  description: string;
  secret?: boolean;
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
  phase?: 'scan' | 'scan-result' | 'scan-error';
  stackId?: string;
  stackName?: string;
}
