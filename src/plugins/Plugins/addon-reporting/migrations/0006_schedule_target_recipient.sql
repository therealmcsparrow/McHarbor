-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Per-target delivery overrides.
--
-- 0005 gave a storage target its own folder. Two things were still fixed
-- for every schedule: the name of the file written into that folder, and
-- the destination a comm channel delivers to. Both are now the
-- operator's, where the provider allows it:
--
--   filename   a pattern (tokens {name} {type} {env} {date} {time}),
--              rendered per run. The extension is still the plugin's,
--              so a file can never claim a format it is not.
--   recipient  an alternative destination for one comm target - the
--              board member this monthly report goes to, from the same
--              channel that mails the ops list every morning. Only
--              accepted for a channel whose adapter declared an
--              overridable field; a Slack webhook has none, because its
--              URL is both the destination and the secret.
--
-- Empty is the pre-existing behaviour in both cases, so every schedule
-- written before this migration keeps doing exactly what it did.
ALTER TABLE plugin_reporting_schedule_targets
    ADD COLUMN filename TEXT NOT NULL DEFAULT '';

ALTER TABLE plugin_reporting_schedule_targets
    ADD COLUMN recipient TEXT NOT NULL DEFAULT '';
