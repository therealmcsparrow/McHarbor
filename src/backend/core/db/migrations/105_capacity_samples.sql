-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS capacity_samples (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    subject_kind    TEXT NOT NULL,
    subject_id      TEXT NOT NULL,
    subject_name    TEXT NOT NULL DEFAULT '',
    sampled_at      TEXT NOT NULL,
    bucket_seconds  INTEGER NOT NULL DEFAULT 3600,
    used_bytes      INTEGER NOT NULL DEFAULT 0,
    total_bytes     INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_capacity_samples_unique
    ON capacity_samples(environment_id, subject_kind, subject_id, sampled_at);

CREATE INDEX IF NOT EXISTS idx_capacity_samples_prune
    ON capacity_samples(subject_kind, sampled_at);

CREATE INDEX IF NOT EXISTS idx_capacity_samples_lookup
    ON capacity_samples(environment_id, subject_kind, sampled_at);
