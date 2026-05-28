-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE environments ADD COLUMN agent_token_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_agent_token_hash
    ON environments(agent_token_hash);
