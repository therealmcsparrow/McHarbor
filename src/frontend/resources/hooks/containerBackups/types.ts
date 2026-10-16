// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { UploadProgress } from './uploadWithProgress';

// BackupSelectionMode mirrors the backend: off captures nothing, full
// captures the whole option, choose captures only the listed paths.
// Paths are prefixes — picking a folder takes everything under it,
// including files added after the plan was saved.
export type BackupSelectionMode = 'off' | 'choose' | 'full';

export type BackupSelection = {
  mode: BackupSelectionMode;
  paths?: string[];
};

// ConfigSection is one selectable part of the container definition, keyed
// by its dotted path into the inspect document.
export type ConfigSection = {
  key: string;
  label: string;
  description: string;
};

export type ContainerBackupOption = {
  key: string;
  type: 'config' | 'image' | 'filesystem' | 'logs' | 'volume' | 'bind';
  label: string;
  description: string;
  default: boolean;
  required: boolean;
  // partial is false for image and logs, which are all-or-nothing; the
  // UI renders those two-state instead of three-state.
  partial?: boolean;
};

export type ContainerBackupOptions = {
  containerId: string;
  containerName: string;
  options: ContainerBackupOption[];
  configSections?: ConfigSection[];
};

export type ContainerBackupPlan = {
  id: string;
  name: string;
  environmentId: string;
  containerId: string;
  containerName: string;
  // containerIdStale is true when the server refreshed the
  // container_id on this read because the stored id no longer
  // matched a live container. The persisted row has already been
  // updated, so the UI just surfaces a small "Re-linked" badge.
  containerIdStale?: boolean;
  storageLocationId?: string;
  storageLocationIds: string[];
  includeConfig: boolean;
  includeLogs: boolean;
  includeFilesystem: boolean;
  includeImage: boolean;
  selectedMounts: string[];
  selections?: Record<string, BackupSelection>;
  logTailLines?: number;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  lastRunAt?: string;
  nextRunAt?: string;
  consistencyMode?: string;
  snapshotConfigId?: string;
  snapshotConfigName?: string;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRun = {
  id: string;
  planId?: string;
  planName?: string;
  operation: 'backup' | 'restore';
  // "upload" marks an archive an operator supplied; absent when McHarbor made it.
  origin?: 'upload';
  sourceRunId?: string;
  environmentId: string;
  containerId: string;
  containerName?: string;
  // See ContainerBackupPlan.containerIdStale.
  containerIdStale?: boolean;
  status: 'running' | 'success' | 'failure' | 'cancelled';
  archivePath?: string;
  archiveSize: number;
  archiveEncryption?: string;
  archiveKeyId?: string;
  requiresSecretKey?: boolean;
  error?: string;
  progressStage?: string;
  progressMessage?: string;
  progressUpdatedAt?: string;
  destinations: ContainerBackupRunDestination[];
  // includedItems is what the run actually captured (backup) or wrote
  // back (restore); availableItems is what was on offer at the time.
  // Both use the "config" / "image" / "filesystem" / "logs" /
  // "mount:<destination>" keys the backup options endpoint mints.
  // scope is the derived label, absent for runs recorded before the
  // server started snapshotting the inventory.
  includedItems?: string[];
  availableItems?: string[];
  scope?: 'full' | 'partial';
  startedAt: string;
  completedAt?: string;
  durationMs: number;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRunDestination = {
  id: string;
  runId: string;
  storageLocationId?: string;
  storageLocationName: string;
  locationType: string;
  status: 'uploading' | 'success' | 'failure';
  path: string;
  error?: string;
  uploadedAt?: string;
  bytesUploaded?: number;
  bytesTotal?: number;
  createdAt: string;
  updatedAt: string;
};

export type ContainerBackupRelinkAllResult = {
  plansChecked: number;
  plansRefreshed: number;
  runsChecked: number;
  runsRefreshed: number;
  refreshedRunIds?: string[];
  refreshedPlanIds?: string[];
};

export type ContainerBackupRestoreInput = {
  id: string;
  secretKey?: string;
  restoreItems: string[];
};

export type ContainerBackupUploadRestoreInput = {
  file: File;
  secretKey?: string;
  onProgress?: (progress: UploadProgress) => void;
};

export type ContainerBackupRestoreResult = {
  runId: string;
  restored: string[];
};

export type ContainerBackupRestoreOption = {
  key: string;
  type: 'image' | 'filesystem' | 'mount' | 'folder' | 'file' | 'config' | 'logs' | 'volume' | 'bind';
  label: string;
  description: string;
  default: boolean;
  required: boolean;
  // path / parentMount / size / truncated are populated for
  // tree-listing entries (returned by the per-mount, per-filesystem,
  // and per-config endpoints). They are absent on the flat
  // restore-options top-level summary.
  path?: string;
  parentMount?: string;
  size?: number;
  truncated?: boolean;
};

export type ContainerBackupRestoreOptions = {
  runId: string;
  items: ContainerBackupRestoreOption[];
  message?: string;
  // truncated / maxChildrenCap are echoed by the per-tree endpoints
  // when the server-side cap was hit.
  truncated?: boolean;
  maxChildrenCap?: number;
};

export type ContainerBackupInput = {
  name: string;
  storageLocationId?: string;
  storageLocationIds: string[];
  includeConfig: boolean;
  includeLogs: boolean;
  includeFilesystem: boolean;
  includeImage: boolean;
  selectedMounts: string[];
  selections?: Record<string, BackupSelection>;
  logTailLines?: number;
  cron?: string;
  enabled?: boolean;
  retentionCount: number;
  retentionDays: number;
  /**
   * How the archive is made consistent: "pause" stops the container,
   * "snapshot" freezes the filesystem and leaves it running, "live"
   * accepts a possibly torn copy. Absent means "pause".
   */
  consistencyMode?: string;
  snapshotConfigId?: string;
};
