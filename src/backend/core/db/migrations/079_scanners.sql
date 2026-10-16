-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- 079_scanners.sql
--
-- Moves the three built-in vulnerability scanners (Trivy, Grype,
-- Clair) out of `modules/scans` and behind the plugin runtime's new
-- `scanner` adapter family, and gives the host somewhere to keep the
-- per-scanner configuration the engines used to read out of the
-- `scanners` settings category.
--
-- Shape follows `audit_log_forwarders` exactly, and for the same
-- reason: a scanner is a *named instance* of an adapter type, not a
-- singleton. One Clair adapter can back a production server and a
-- staging one; two Trivy rows can carry different severity floors.
-- The old model — a boolean per engine plus one global Clair URL —
-- could express none of that.
--
-- What does NOT move: `scans`, `vulnerabilities`, retention, the scan
-- history page, the summary rollups. Those are host-owned and stay
-- that way; a plugin uninstall must never take scan history with it.

CREATE TABLE IF NOT EXISTS scanner_configs (
    id            TEXT PRIMARY KEY,
    -- Operator-facing label. The only thing separating two instances
    -- of the same engine in the Scanners tab, so it is required.
    name          TEXT NOT NULL,
    -- Binds the row to a registered ImageScannerAdapterSpec. Keeps
    -- the exact keys the built-in scanners used ("trivy", "grype",
    -- "clair") so `scans.scanner` values already written to history
    -- still name something the tab can resolve.
    scanner_type  TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0, 1)),
    -- Exactly one row should carry is_default = 1. The host elects a
    -- replacement when the default row is deleted or disabled rather
    -- than enforcing it with a constraint, because a partial unique
    -- index would make "disable everything" impossible.
    is_default    INTEGER NOT NULL DEFAULT 0 CHECK(is_default IN (0, 1)),
    -- The adapter's declared ConfigSchema values as a JSON object.
    -- Plaintext for ordinary fields, ciphertext for secrets. The host
    -- never interprets the keys.
    config        TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at    TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_scanner_configs_enabled
    ON scanner_configs(enabled, scanner_type);

CREATE INDEX IF NOT EXISTS idx_scanner_configs_default
    ON scanner_configs(is_default);


-- Carry the existing settings across.
--
-- An install that had Trivy on and a Clair URL configured must come
-- out of this upgrade still scanning, with the same engines enabled
-- and the same Clair endpoint. Anything less makes an upgrade a
-- silent capability loss, which is the one thing a migration must not
-- do.
--
-- The `scanners` settings rows are deliberately NOT deleted here. If
-- this release is rolled back, the old code path finds its settings
-- intact. Pruning them is a follow-up once the plugin path has run in
-- production.
--
-- The ids are fixed rather than generated so re-running the migration
-- on a partially-migrated database is a no-op: `INSERT OR IGNORE` on
-- a known primary key cannot produce a duplicate scanner.

-- Trivy. Enabled when the old flag said so, defaulting to on — which
-- is what DefaultScannerSettings() returned for an install that never
-- touched the setting, and the reason a fresh install could scan
-- without configuring anything.
INSERT OR IGNORE INTO scanner_configs (id, name, scanner_type, enabled, is_default, config)
SELECT
    'scannercfgtrivy0000000000',
    'Trivy',
    'trivy',
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_trivy_enabled'),
        'true') = 'true' THEN 1 ELSE 0 END,
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_default'),
        'trivy') = 'trivy'
      AND COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_trivy_enabled'),
        'true') = 'true'
    THEN 1 ELSE 0 END,
    '{}';

-- Grype. Off by default, matching the old defaults.
INSERT OR IGNORE INTO scanner_configs (id, name, scanner_type, enabled, is_default, config)
SELECT
    'scannercfggrype0000000000',
    'Grype',
    'grype',
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_grype_enabled'),
        'false') = 'true' THEN 1 ELSE 0 END,
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_default'),
        'trivy') = 'grype'
      AND COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_grype_enabled'),
        'false') = 'true'
    THEN 1 ELSE 0 END,
    '{}';

-- Clair. Only created when a server URL was actually configured: a
-- Clair row with no endpoint is a scanner that cannot scan, and
-- seeding one would put a permanently broken entry in every operator's
-- Scanners tab. The URL is copied into the adapter's `server_url`
-- config key.
INSERT OR IGNORE INTO scanner_configs (id, name, scanner_type, enabled, is_default, config)
SELECT
    'scannercfgclair0000000000',
    'Clair',
    'clair',
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_clair_enabled'),
        'false') = 'true' THEN 1 ELSE 0 END,
    CASE WHEN COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_default'),
        'trivy') = 'clair'
      AND COALESCE(
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_clair_enabled'),
        'false') = 'true'
    THEN 1 ELSE 0 END,
    json_object('server_url',
        (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_clair_url'))
WHERE COALESCE(
    (SELECT value FROM settings WHERE category = 'scanners' AND key = 'scanner_clair_url'),
    '') <> '';

-- Fallback election. Every path above can leave zero default rows —
-- `scanner_default` naming clair on an install that never set a Clair
-- URL, for instance. A scan started with no scanner named would then
-- have nothing to resolve to, so promote the first enabled row.
UPDATE scanner_configs
SET is_default = 1
WHERE id = (
    SELECT id FROM scanner_configs
    WHERE enabled = 1
    ORDER BY created_at ASC, id ASC
    LIMIT 1
)
AND NOT EXISTS (SELECT 1 FROM scanner_configs WHERE is_default = 1);


-- The gate `bundled.go` reads to decide whether to auto-install
-- `scanner-trivy`.
--
-- Unlike the Kubernetes entry this is set on a fresh install too, and
-- deliberately: Trivy was on by default before the engines became
-- plugins, so "the host would be taking a capability away" is true of
-- an upgrade *and* of a new install that expects to be able to scan
-- out of the box. The Trivy row above exists in both cases for the
-- same reason.
--
-- `bundled.go` still installs at most once — its DoneSetting makes a
-- deliberate uninstall permanent rather than undone by the next
-- restart.
INSERT INTO settings (id, key, value, category, updated_at)
SELECT 'set_scanner_configs_migrated', 'scanner_configs_migrated', '1', 'scanners', CURRENT_TIMESTAMP
WHERE EXISTS (SELECT 1 FROM scanner_configs)
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'scanner_configs_migrated');


-- Action catalog entries for the plugin-backed scanner admin surface.
--
-- Severity: saving and deleting are `warning` because changing which
-- engine scans an estate — or switching scanning off — is exactly the
-- configuration change an auditor wants flagged. Testing is `info`: it
-- proves the engine works and changes nothing.
--
-- Conflict target is `action`, the column carrying the UNIQUE
-- constraint. Targeting `id` is what silently swallowed the forwarder
-- entries in the migration this one is modelled on.

INSERT INTO audit_action_catalog (id, module, action, severity_default, description)
VALUES
  ('audit_action_080', 'scans', 'scans.scanner.saved',   'warning', 'Vulnerability scanner configuration changed'),
  ('audit_action_081', 'scans', 'scans.scanner.deleted', 'warning', 'Vulnerability scanner removed'),
  ('audit_action_082', 'scans', 'scans.scanner.tested',  'info',    'Vulnerability scanner test run')
ON CONFLICT(action) DO NOTHING;
