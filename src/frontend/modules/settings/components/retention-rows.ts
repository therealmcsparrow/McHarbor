// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The table of purgeable logs, plus the defaults the retention card falls
// back to before the server answers.
//
// This lives beside the card rather than inside it because three files now
// need it: the card that renders the grid, the row component that renders one
// entry, and the confirm dialog that has to name the log being cleared.

/** The purgeable logs, keyed the way the retention API names them. */
export type RetentionKey = 'audit' | 'activity' | 'lifecycle' | 'scans' | 'apiKeyUsage';

// One row per purgeable log. Keeping the copy keys in a table means the
// five rows render from a single block of JSX instead of five near-identical
// copies, and a new log type is one entry rather than a new section.
export const RETENTION_ROWS: {
  key: RetentionKey;
  labelKey: string;
  descriptionKey: string;
  confirmTitleKey: string;
  confirmDescriptionKey: string;
}[] = [
  {
    key: 'audit',
    labelKey: 'retention.auditDays',
    descriptionKey: 'retention.auditDaysDescription',
    confirmTitleKey: 'retention.clearAuditTitle',
    confirmDescriptionKey: 'retention.clearAuditDescription',
  },
  {
    key: 'activity',
    labelKey: 'retention.activityDays',
    descriptionKey: 'retention.activityDaysDescription',
    confirmTitleKey: 'retention.clearActivityTitle',
    confirmDescriptionKey: 'retention.clearActivityDescription',
  },
  {
    key: 'lifecycle',
    labelKey: 'retention.lifecycleDays',
    descriptionKey: 'retention.lifecycleDaysDescription',
    confirmTitleKey: 'retention.clearLifecycleTitle',
    confirmDescriptionKey: 'retention.clearLifecycleDescription',
  },
  {
    key: 'scans',
    labelKey: 'retention.scanDays',
    descriptionKey: 'retention.scanDaysDescription',
    confirmTitleKey: 'retention.clearScansTitle',
    confirmDescriptionKey: 'retention.clearScansDescription',
  },
  {
    key: 'apiKeyUsage',
    labelKey: 'retention.apiKeyUsageDays',
    descriptionKey: 'retention.apiKeyUsageDaysDescription',
    confirmTitleKey: 'retention.clearAPIKeyUsageTitle',
    confirmDescriptionKey: 'retention.clearAPIKeyUsageDescription',
  },
];

export const DEFAULT_DAYS: Record<RetentionKey, number> = {
  audit: 90,
  activity: 30,
  lifecycle: 30,
  scans: 90,
  apiKeyUsage: 90,
};

// Metric history is not a purgeable log, so it does not belong in
// RETENTION_ROWS: it has no "clear now", and its first horizon is measured in
// hours rather than days. It gets its own block below the log grid.
export const DEFAULT_METRICS = { rawHours: 24, rollupDays: 30, diskEnabled: true };

// Off by default: on a measured install, exec events were 95.4% of all stored
// events (~5.2 GB in eight days). Health outcomes are recorded separately in
// container_health_probes, so nothing is lost while this is off.
export const DEFAULT_RECORD_EXEC = false;

/** The slice of a purge mutation the retention card and its rows use. */
export type PurgeMutationLike = {
  isPending: boolean;
  mutate: (variables: void, options?: { onSuccess?: () => void }) => void;
};

/** The five day counts as the retention API reports them. */
export type RetentionDaysSource = {
  auditRetentionDays: number;
  activityRetentionDays: number;
  lifecycleRetentionDays: number;
  scanRetentionDays: number;
  apiKeyUsageRetentionDays: number;
};

/**
 * Projects a retention response onto the RetentionKey map the form uses.
 *
 * Two call sites need this and they must not drift: one seeds the draft after
 * the query resolves, the other reads the saved horizons that "clear now"
 * acts on. A key added to one copy and not the other would purge against the
 * wrong number, silently.
 */
export function daysFromRetention(retention: RetentionDaysSource | undefined): Record<RetentionKey, number> {
  return {
    audit: retention?.auditRetentionDays ?? 0,
    activity: retention?.activityRetentionDays ?? 0,
    lifecycle: retention?.lifecycleRetentionDays ?? 0,
    scans: retention?.scanRetentionDays ?? 0,
    apiKeyUsage: retention?.apiKeyUsageRetentionDays ?? 0,
  };
}
