-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE host_metrics ADD COLUMN net_rx INTEGER;
ALTER TABLE host_metrics ADD COLUMN net_tx INTEGER;
ALTER TABLE host_metrics ADD COLUMN block_read INTEGER;
ALTER TABLE host_metrics ADD COLUMN block_write INTEGER;
