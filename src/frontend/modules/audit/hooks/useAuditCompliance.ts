// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { DateRange } from '@resources/utils/date-range';
import type { SearchMode } from '@resources/utils/search-filter';

export type AuditExportFormat = 'json' | 'csv' | 'jsonl';

export const AUDIT_EXPORT_FIELDS = [
  'id',
  'timestamp',
  'severity',
  'userId',
  'username',
  'action',
  'entityType',
  'entityId',
  'entityName',
  'environmentId',
  'environmentName',
  'ipAddress',
  'apiKeyId',
  'apiKeyName',
  'details',
] as const;

/**
 * Build the query parameters for a signed audit export.
 *
 * Only keys with a value are set: the backend treats an absent filter
 * and an empty one differently, and sending `action=` would filter to
 * events whose action is the empty string rather than to all events.
 */
export function auditExportParams(
  format: AuditExportFormat,
  action: string,
  entityType: string,
  range: DateRange,
  envId?: string | null,
  search = '',
  mode: SearchMode = 'contains',
  extra: { actor?: string; severities?: string[]; fields?: string[] } = {},
) {
  const params: Record<string, string> = { format };
  if (action) params.action = action;
  if (entityType) params.entity_type = entityType;
  if (envId) params.env = envId;
  if (range.from) params.from = range.from.toISOString();
  if (range.to) params.to = range.to.toISOString();
  // `regex` is not a filter the API can apply, so an export in that mode is
  // deliberately unfiltered rather than filtered by a query it cannot honour.
  const serverSearch = mode === 'regex' ? '' : search.trim();
  if (serverSearch) {
    params.search = serverSearch;
    params.search_mode = mode;
  }
  if (extra.actor?.trim()) params.actor = extra.actor.trim();
  if (extra.severities && extra.severities.length > 0) params.severity = extra.severities.join(',');
  if (extra.fields && extra.fields.length > 0) params.fields = extra.fields.join(',');
  return params;
}
