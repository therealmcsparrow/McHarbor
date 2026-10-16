-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE desired_states ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN subject_id TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN spec TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN plugin_env TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_desired_states_source ON desired_states(source, kind);
