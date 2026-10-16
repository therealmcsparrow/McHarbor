-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE plugin_addon_compute_networks ADD COLUMN ipv6_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plugin_addon_compute_networks ADD COLUMN host_access_ipv6 TEXT;
ALTER TABLE plugin_addon_compute_networks ADD COLUMN host_link_name TEXT;
