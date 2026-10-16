-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE audit_logs ADD COLUMN api_key_id TEXT;
ALTER TABLE audit_logs ADD COLUMN api_key_name TEXT;
CREATE INDEX IF NOT EXISTS idx_audit_logs_api_key ON audit_logs(api_key_id);
