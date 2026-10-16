-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE desired_states ADD COLUMN kind TEXT NOT NULL DEFAULT 'container';
ALTER TABLE desired_states ADD COLUMN stack_name TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_states ADD COLUMN last_report TEXT;
