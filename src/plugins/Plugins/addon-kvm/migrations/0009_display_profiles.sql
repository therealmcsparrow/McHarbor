-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

CREATE TABLE IF NOT EXISTS plugin_addon_kvm_display_profiles (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL UNIQUE,
    description              TEXT,
    builtin                  INTEGER NOT NULL DEFAULT 0,
    image_compression        TEXT NOT NULL,
    jpeg_wan_compression     TEXT NOT NULL,
    zlib_glz_wan_compression TEXT NOT NULL,
    streaming_video          TEXT NOT NULL,
    playback_compression     TEXT NOT NULL,
    agent_mouse              TEXT NOT NULL,
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);

INSERT OR IGNORE INTO plugin_addon_kvm_display_profiles
    (id, name, description, builtin, image_compression, jpeg_wan_compression,
     zlib_glz_wan_compression, streaming_video, playback_compression, agent_mouse,
     created_at, updated_at)
VALUES
    ('builtin-fast-lan', 'Fast LAN',
     'Uncompressed images: the most bandwidth, the least work for the browser.', 1,
     'off', 'never', 'never', 'filter', 'on', 'on',
     '2026-10-01T00:00:00Z', '2026-10-01T00:00:00Z'),
    ('builtin-balanced', 'Balanced',
     'LZ for desktop content, JPEG on a slow link.', 1,
     'auto_lz', 'auto', 'never', 'filter', 'on', 'on',
     '2026-10-01T00:00:01Z', '2026-10-01T00:00:01Z'),
    ('builtin-low-bandwidth', 'Low bandwidth',
     'Lossy JPEG and MJPEG streaming everywhere: the fewest bytes.', 1,
     'auto_lz', 'always', 'never', 'all', 'on', 'on',
     '2026-10-01T00:00:02Z', '2026-10-01T00:00:02Z');
