-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE INDEX IF NOT EXISTS idx_container_events_env_time ON container_events(environment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_scans_created ON scans(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_scans_env_created ON scans(environment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_started ON workflow_runs(workflow_id, started_at DESC);
