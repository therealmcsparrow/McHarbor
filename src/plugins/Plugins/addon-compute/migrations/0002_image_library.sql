-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE plugin_addon_compute_images
    ADD COLUMN origin TEXT NOT NULL DEFAULT 'catalog';

ALTER TABLE plugin_addon_compute_images
    ADD COLUMN library_file TEXT;

ALTER TABLE plugin_addon_compute_images
    ADD COLUMN uploaded_by TEXT;

ALTER TABLE plugin_addon_compute_images
    ADD COLUMN uploaded_at TEXT;

CREATE INDEX IF NOT EXISTS idx_plugin_addon_compute_images_origin
    ON plugin_addon_compute_images(origin);
