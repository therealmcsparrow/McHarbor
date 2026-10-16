-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Scope the delivery ledger to one forwarder instance.
--
-- The host now allows several SIEM destinations — a production
-- collector and a staging one, say — each a row in
-- audit_log_forwarders with its own id. This table is the dedup and
-- retry ledger for ForwardPending, so without an instance column the
-- first destination to deliver an event would mark it delivered for
-- every other destination too, and the second collector would receive
-- nothing. The id arrives in the config map as `__forwarder_id`.
--
-- DEFAULT '' keeps the insert in the previous plugin build valid until
-- the new binary lands; the backfill below then attaches the existing
-- history to the one forwarder that could have written it.

ALTER TABLE plugin_logging_siem_delivery_history
    ADD COLUMN forwarder_id TEXT NOT NULL DEFAULT '';

-- Before this migration only one SIEM forwarder could exist, so every
-- historical row belongs to whichever row survives in
-- audit_log_forwarders. Without this the ledger would look empty to
-- that instance and it would re-ship its entire backlog to a collector
-- that already has it.
UPDATE plugin_logging_siem_delivery_history
SET forwarder_id = COALESCE(
        (SELECT id FROM audit_log_forwarders WHERE forwarder_type = 'siem' ORDER BY created_at ASC LIMIT 1),
        ''
    )
WHERE forwarder_id = '';

CREATE INDEX IF NOT EXISTS idx_plugin_logging_siem_delivery_forwarder
    ON plugin_logging_siem_delivery_history(forwarder_id, audit_log_id, status);
