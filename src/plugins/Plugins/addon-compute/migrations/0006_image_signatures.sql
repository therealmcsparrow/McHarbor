-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

UPDATE plugin_addon_compute_images
   SET signature_url = checksum_url || '.gpg'
 WHERE id IN ('img-ubuntu-2404-amd64', 'img-ubuntu-2204-amd64', 'img-ubuntu-2404-rootfs-amd64')
   AND signature_url IS NULL
   AND checksum_url IS NOT NULL;

UPDATE plugin_addon_compute_images
   SET signature_url = checksum_url
 WHERE id = 'img-fedora-41-amd64'
   AND signature_url IS NULL
   AND checksum_url IS NOT NULL;

UPDATE plugin_addon_compute_images
   SET signature_url = source_url || '.asc'
 WHERE id = 'img-alpine-321-rootfs-amd64'
   AND signature_url IS NULL;
