-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Delivery history for the syslog forwarder.
--
-- This table is the plugin's only storage. Config lives in the host's
-- `audit_log_forwarders` row so the Logging tab can render the form
-- generically; history lives here so uninstalling the plugin takes
-- its bookkeeping with it.
--
-- The table doubles as the dedup and retry ledger: ForwardPending
-- selects audit events with no successful row here and fewer than
-- max_retries rows total. That is why audit_log_id is indexed
-- together with status — the join runs once per five-second tick.
--
-- protocol records the transport each attempt actually used, which
-- matters more here than in the SIEM twin: a "success" over UDP means
-- only that the datagram left this host, while one over TLS means the
-- collector accepted it. Storing the protocol per row keeps that
-- distinction visible in the Logging tab instead of flattening two
-- very different guarantees into one green badge.
--
-- audit_log_id is intentionally NOT a foreign key to audit_logs: the
-- host prunes audit rows on a retention schedule, and a cascade would
-- delete the evidence that an event was shipped off-box before it was
-- pruned locally — the one record an auditor actually wants. NULL
-- means the row records a Test send, which has no audit event.
--
-- There is no status_code column. Syslog has no protocol-level
-- response, and a column that is always zero invites a reader to
-- believe it means something.

CREATE TABLE IF NOT EXISTS plugin_logging_syslog_delivery_history (
    id              TEXT PRIMARY KEY,
    audit_log_id    TEXT,
    status          TEXT NOT NULL CHECK(status IN ('success', 'failed')),
    attempt         INTEGER NOT NULL DEFAULT 1,
    protocol        TEXT NOT NULL DEFAULT 'tls' CHECK(protocol IN ('udp', 'tcp', 'tls')),
    error           TEXT NOT NULL DEFAULT '',
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_plugin_logging_syslog_delivery_log
    ON plugin_logging_syslog_delivery_history(audit_log_id, status);

CREATE INDEX IF NOT EXISTS idx_plugin_logging_syslog_delivery_created
    ON plugin_logging_syslog_delivery_history(created_at DESC);
