-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE users ADD COLUMN principal_kind TEXT NOT NULL DEFAULT 'user';

CREATE INDEX IF NOT EXISTS idx_users_principal_kind ON users(principal_kind);

ALTER TABLE runbooks ADD COLUMN automation_principal_id TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_on_alert INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runbooks ADD COLUMN automation_cron TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_since TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_next_run_at TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_last_run_at TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE runbooks ADD COLUMN automation_updated_by TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_runbooks_automation ON runbooks(automation_principal_id, automation_next_run_at);

ALTER TABLE runbook_runs ADD COLUMN trigger_kind TEXT NOT NULL DEFAULT 'manual';

CREATE INDEX IF NOT EXISTS idx_runbook_runs_principal ON runbook_runs(started_by, trigger_kind, started_at);
CREATE INDEX IF NOT EXISTS idx_runbook_runs_alert_runbook ON runbook_runs(runbook_id, alert_event_id);
