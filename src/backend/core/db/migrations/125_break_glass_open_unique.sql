-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

UPDATE break_glass_sessions SET ended_at = started_at, ended_by = 'migration'
 WHERE ended_at IS NULL
   AND id NOT IN (SELECT id FROM (SELECT MAX(id) AS id FROM break_glass_sessions WHERE ended_at IS NULL GROUP BY user_id) AS newest);

CREATE UNIQUE INDEX IF NOT EXISTS ux_break_glass_sessions_open ON break_glass_sessions(user_id) WHERE ended_at IS NULL;
