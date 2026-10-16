-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- The ad-hoc report type stores the question that produced it and the
-- resolved source plan. The plan is pinned so a regenerate re-runs the
-- authoring pass against fresh data without re-planning, which is what
-- keeps a scheduled ad-hoc report comparable period to period.

ALTER TABLE plugin_reporting_reports ADD COLUMN prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE plugin_reporting_reports ADD COLUMN plan TEXT NOT NULL DEFAULT '';
