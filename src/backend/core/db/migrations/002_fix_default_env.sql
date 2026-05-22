-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Remove stale "default" environment row that causes infinite recursion
-- in ClientPool.resolveDefault() -> resolveConnection("default") -> resolveDefault()
DELETE FROM host_metrics WHERE environment_id = 'default';
DELETE FROM environments WHERE id = 'default';
