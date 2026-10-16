// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export interface SnapshotSchedule {
  id: string;
  vmId: string;
  name: string;
  cron: string;
  enabled: boolean;
  keepCount: number;
  withMemory: boolean;
  pauseWithoutAgent: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SnapshotRun {
  id: string;
  scheduleId: string;
  vmId: string;
  status: 'running' | 'success' | 'failure';
  snapshotId?: string;
  snapshotName?: string;
  pruned: number;
  error?: string;
  startedAt: string;
  completedAt?: string;
}

export interface SnapshotScheduleView {
  schedules: SnapshotSchedule[];
  runs: SnapshotRun[];
}

export interface ScheduleInput {
  name: string;
  cron: string;
  enabled?: boolean;
  keepCount?: number;
  withMemory?: boolean;
  pauseWithoutAgent?: boolean;
}
