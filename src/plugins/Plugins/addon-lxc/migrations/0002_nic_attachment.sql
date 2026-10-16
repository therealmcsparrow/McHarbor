-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE plugin_addon_lxc_nics ADD COLUMN attach_kind TEXT;
ALTER TABLE plugin_addon_lxc_nics ADD COLUMN parent_link TEXT;
ALTER TABLE plugin_addon_lxc_nics ADD COLUMN link_mode TEXT;
