-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS storage_location_oauth_states (
    state               TEXT PRIMARY KEY,
    storage_location_id TEXT NOT NULL,
    provider            TEXT NOT NULL,
    expires_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    FOREIGN KEY(storage_location_id) REFERENCES storage_locations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_storage_location_oauth_states_location ON storage_location_oauth_states(storage_location_id);
CREATE INDEX IF NOT EXISTS idx_storage_location_oauth_states_expires ON storage_location_oauth_states(expires_at);
