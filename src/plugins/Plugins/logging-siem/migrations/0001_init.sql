-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Delivery history for the SIEM HTTP forwarder.
--
-- This table is the plugin's only storage. Config lives in the host's
-- `audit_log_forwarders` row so the Logging tab can render the form
-- generically and encrypt secrets through the host's one
-- encryption.Service; history lives here so uninstalling the plugin
-- takes its bookkeeping with it.
--
-- The table doubles as the dedup and retry ledger: ForwardPending
-- selects audit events with no successful row here and fewer than
-- max_retries rows total. That is why audit_log_id is indexed
-- together with status — the join runs once per five-second tick.
--
-- audit_log_id is intentionally NOT a foreign key to audit_logs. The
-- host prunes audit rows on a retention schedule, and a FK would
-- either block the prune or cascade away the evidence that an event
-- was successfully shipped off-box before it was pruned locally —
-- which is the one record a compliance auditor actually wants.
-- NULL means the row records a Test send, which has no audit event.

CREATE TABLE IF NOT EXISTS plugin_logging_siem_delivery_history (
    id              TEXT PRIMARY KEY,
    audit_log_id    TEXT,
    status          TEXT NOT NULL CHECK(status IN ('success', 'failed')),
    attempt         INTEGER NOT NULL DEFAULT 1,
    status_code     INTEGER NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_plugin_logging_siem_delivery_log
    ON plugin_logging_siem_delivery_history(audit_log_id, status);

CREATE INDEX IF NOT EXISTS idx_plugin_logging_siem_delivery_created
    ON plugin_logging_siem_delivery_history(created_at DESC);
