-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Records which configured AI connection generated or polished a
-- report, so the library can show which AI wrote it and a pinned
-- ad-hoc plan can replay against the same connection (see AdHocPlan in
-- adhoc_plan.go) instead of silently drifting to whatever the current
-- default happens to be.

ALTER TABLE plugin_reporting_reports ADD COLUMN ai_connection_id TEXT NOT NULL DEFAULT '';
