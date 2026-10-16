// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MetricsScrapeCard } from './MetricsScrapeCard';
import { RetentionSettingsCard } from './RetentionSettingsCard';
import { BackupLogRetentionCard } from '@resources/components/settings/BackupLogRetentionCard';

/**
 * Settings → Data retention.
 *
 * Retention used to be the fifth card at the bottom of General, under
 * language, timezone, date format and auto-refresh. Those four are how the
 * app looks to the person reading it; retention is how much data the install
 * keeps and what gets deleted. Different question, different audience —
 * everyone has an opinion about date formats, only an operator sets a purge
 * horizon — so it now answers on its own, in the Monitoring group next to
 * Alerts.
 *
 *   1. Logs and metrics — the audit preset, five purgeable logs, metric
 *      history, and whether exec events are recorded at all.
 *   2. Backup logs      — the same question for backup run history.
 *
 * The backup card also renders on the Backups page, where it is closer to the
 * plans it describes. Both write through the same whole-object PUT that
 * spreads the server's current values, so neither copy can clobber the other.
 */
export function RetentionTab() {
  return (
    <div className="space-y-6">
      <RetentionSettingsCard />
      <BackupLogRetentionCard />
      <MetricsScrapeCard />
    </div>
  );
}
