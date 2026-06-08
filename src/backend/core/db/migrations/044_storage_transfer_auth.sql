-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

ALTER TABLE storage_locations ADD COLUMN auth_method TEXT;
ALTER TABLE storage_locations ADD COLUMN tls_mode TEXT;
ALTER TABLE storage_locations ADD COLUMN passive_mode INTEGER NOT NULL DEFAULT 1;
ALTER TABLE storage_locations ADD COLUMN private_key TEXT;
ALTER TABLE storage_locations ADD COLUMN passphrase TEXT;
ALTER TABLE storage_locations ADD COLUMN ca_certificate TEXT;
ALTER TABLE storage_locations ADD COLUMN client_certificate TEXT;
ALTER TABLE storage_locations ADD COLUMN client_key TEXT;
