// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// queryKey prefix kept stable so future hooks (e.g. a dependency
// graph hook) can share invalidations.
export const PLUGIN_QUERY_KEY = 'appstore-plugins' as const;
