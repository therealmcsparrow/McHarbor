// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** The four SQL backends McHarbor can run on. */
export type DatabaseDriver = 'sqlite' | 'postgres' | 'mysql' | 'mssql';

/** Where the active connection came from. Mirrors core/dbconfig.Source. */
export type DatabaseSource = 'env' | 'secret' | 'default';

/**
 * A target database as the form describes it.
 *
 * Either the structured fields or `dsn` — never both. `dsn` is the escape
 * hatch for a managed database whose connection string carries options the
 * form does not model, and when it is set the backend ignores every other
 * field.
 */
export type DatabaseConnectionInput = {
  driver: DatabaseDriver;
  host?: string;
  port?: number;
  name?: string;
  user?: string;
  password?: string;
  sslMode?: string;
  trustServerCertificate?: boolean;
  dsn?: string;
  path?: string;
};

/** The active database, as GET /database/config returns it. */
export type DatabaseConfig = {
  driver: DatabaseDriver;
  source: DatabaseSource;
  /** Redacted: the password is replaced with *** before it leaves the backend. */
  target: string;
  /** True when MCHARBOR_DB_DRIVER is set, which makes the UI read-only. */
  envPinned: boolean;
  serverVersion?: string;
  tables: number;
  rows: number;
  sizeBytes?: number;
  configured: boolean;
  updatedAt?: string;
  availableDrivers: DatabaseDriver[];
  restartSupported: boolean;
};

/** One line of the step-by-step connection test. */
export type DatabaseTestStep = {
  id: 'validate' | 'resolve' | 'connect' | 'version' | 'privileges' | 'schema' | 'encoding';
  status: 'pass' | 'warn' | 'fail' | 'skip';
  /** An i18n code for the failure cause, when the backend could name one. */
  code?: string;
  /** The driver's own message, with credentials redacted. */
  detail?: string;
  durationMs: number;
};

export type DatabaseTestResult = {
  ok: boolean;
  driver: DatabaseDriver;
  target: string;
  serverVersion?: string;
  steps: DatabaseTestStep[];
  existingTables: number;
};

/** The phases a migration moves through, in order. */
export type MigrationPhase =
  | 'queued'
  | 'prepare'
  | 'schema'
  | 'inventory'
  | 'copy'
  | 'verify'
  | 'catchup'
  | 'persist'
  | 'restart'
  | 'done'
  | 'failed'
  | 'cancelled';

export type MigrationTable = {
  table: string;
  rows: number;
  total: number;
  durationMs: number;
  done: boolean;
};

export type CleanupFile = {
  path: string;
  bytes: number;
};

/** GET /database/cleanup — the old SQLite file left on disk, if any. */
export type CleanupStatus = {
  available: boolean;
  path?: string;
  files?: CleanupFile[];
  totalBytes: number;
  migratedAt?: string;
  /** Why the action is unavailable: none | in_use | missing | unreadable. */
  reason?: 'none' | 'in_use' | 'missing' | 'unreadable';
};

export type CleanupResult = {
  removed: string[];
  bytes: number;
};

export type MigrationStatus = {
  id: string;
  running: boolean;
  phase: MigrationPhase;
  fromDriver: DatabaseDriver;
  toDriver: DatabaseDriver;
  toTarget: string;
  currentTable?: string;
  tableIndex: number;
  tableTotal: number;
  rowsCopied: number;
  rowsTotal: number;
  /** Rows the catch-up passes moved after the bulk copy. Normally zero. */
  rowsCaughtUp: number;
  /** Which catch-up pass is running, 1-based. */
  deltaPass?: number;
  tables: MigrationTable[];
  steps?: DatabaseTestStep[];
  warnings?: string[];
  error?: string;
  errorDetail?: string;
  restartScheduled: boolean;
  restartRequired: boolean;
  startedAt: string;
  finishedAt?: string;
};

/** GET /database/setup/status — drives the first-run screen. */
export type DatabaseSetupStatus = {
  needsSetup: boolean;
  configured: boolean;
  envPinned: boolean;
  driver: DatabaseDriver;
  availableDrivers: DatabaseDriver[];
  restartSupported: boolean;
};

export type DatabaseSetupApplyResult = {
  driver: DatabaseDriver;
  target: string;
  restartScheduled: boolean;
  restartRequired: boolean;
};

/** The phases during which the application must not be navigated away from. */
export const ACTIVE_MIGRATION_PHASES: readonly MigrationPhase[] = [
  'queued',
  'prepare',
  'schema',
  'inventory',
  'copy',
  'verify',
  'catchup',
  'persist',
  'restart',
];

/** Default port per driver, used to prefill the form. */
export const DEFAULT_PORTS: Record<DatabaseDriver, number> = {
  sqlite: 0,
  postgres: 5432,
  mysql: 3306,
  mssql: 1433,
};

/**
 * SSL choices per driver. Postgres speaks libpq's sslmode; MySQL speaks
 * go-sql-driver's `tls` parameter. They are different vocabularies for the
 * same idea, so the picker renders whichever the chosen driver uses.
 */
export const SSL_OPTIONS: Record<DatabaseDriver, string[]> = {
  sqlite: [],
  postgres: ['require', 'verify-full', 'verify-ca', 'prefer', 'disable'],
  mysql: ['preferred', 'true', 'skip-verify', 'false'],
  mssql: ['true', 'strict', 'false', 'disable'],
};

export * from './check-types';
