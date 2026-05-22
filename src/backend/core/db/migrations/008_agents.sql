-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN agent_token TEXT;
ALTER TABLE environments ADD COLUMN agent_status TEXT DEFAULT 'disconnected';
ALTER TABLE environments ADD COLUMN agent_version TEXT;
ALTER TABLE environments ADD COLUMN agent_hostname TEXT;
ALTER TABLE environments ADD COLUMN agent_os TEXT;
ALTER TABLE environments ADD COLUMN agent_arch TEXT;
ALTER TABLE environments ADD COLUMN agent_last_seen TEXT;
CREATE INDEX IF NOT EXISTS idx_environments_agent_token ON environments(agent_token);
