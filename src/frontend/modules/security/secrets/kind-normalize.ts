// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export function normalizeKind(raw: string): string {
  const slug = raw.trim().toLowerCase().replace(/\s+/g, '_');
  return /^[a-z][a-z0-9_]{0,31}$/.test(slug) ? slug : '';
}
