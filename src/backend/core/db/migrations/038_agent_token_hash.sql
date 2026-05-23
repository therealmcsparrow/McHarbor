ALTER TABLE environments ADD COLUMN agent_token_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_environments_agent_token_hash
    ON environments(agent_token_hash);
