-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_env_sev_time
    ON lifecycle_events(environment_id, severity, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_env_scope_time
    ON audit_logs(COALESCE(environment_id, ''), timestamp DESC);
