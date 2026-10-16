-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE plugin_addon_lxc_nics ADD COLUMN static_ipv6 TEXT;
ALTER TABLE plugin_addon_lxc_nics ADD COLUMN static_gw6 TEXT;
