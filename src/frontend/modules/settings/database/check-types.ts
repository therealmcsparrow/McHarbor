// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { DatabaseDriver } from './types';

export type CheckFamily = 'integrity' | 'orphans' | 'schema' | 'bloat';

export type CheckSeverity = 'ok' | 'info' | 'warn' | 'fail' | 'skip';

export type CheckPhase =
  | 'queued'
  | 'inventory'
  | 'integrity'
  | 'orphans'
  | 'schema'
  | 'bloat'
  | 'done'
  | 'failed'
  | 'cancelled';

export type CheckFinding = {
  id: string;
  key: string;
  family: CheckFamily;
  severity: CheckSeverity;
  table?: string;
  column?: string;
  refTable?: string;
  refColumn?: string;
  count?: number;
  bytes?: number;
  reason?: string;
  detail?: string;
  remedy?: string;
  destructive?: boolean;
  index?: string;
};

export type DatabaseCheckStatus = {
  id: string;
  running: boolean;
  phase: CheckPhase;
  driver: DatabaseDriver;
  currentTable?: string;
  tableIndex: number;
  tableTotal: number;
  findings: CheckFinding[];
  counts: Record<string, number>;
  error?: string;
  errorDetail?: string;
  startedAt: string;
  finishedAt?: string;
};

export type RepairOutcome = {
  findingId: string;
  remedy: string;
  ok: boolean;
  rows: number;
  code?: string;
  detail?: string;
  skipped?: boolean;
};

export type RepairResponse = {
  outcomes: RepairOutcome[];
  rows: number;
};

export const ACTIVE_CHECK_PHASES: readonly CheckPhase[] = [
  'queued',
  'inventory',
  'integrity',
  'orphans',
  'schema',
  'bloat',
];
