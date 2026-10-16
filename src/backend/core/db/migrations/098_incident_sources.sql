-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE incidents ADD COLUMN subject_id TEXT NOT NULL DEFAULT '';
ALTER TABLE incidents ADD COLUMN source TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_incidents_source_subject ON incidents(source, subject_type, subject_id, status);
