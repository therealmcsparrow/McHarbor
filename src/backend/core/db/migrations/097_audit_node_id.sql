-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE audit_logs ADD COLUMN node_id TEXT;
CREATE INDEX IF NOT EXISTS idx_audit_logs_node_id ON audit_logs(node_id);
