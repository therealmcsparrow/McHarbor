// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** Must stay equal to `envredact.Marker` in src/backend/core/envredact/envredact.go. */
export const ENV_REDACTION_MARKER = '••••••••';

export function isRedactedValue(value: string | undefined): boolean {
  return value === ENV_REDACTION_MARKER;
}

/**
 * Every reveal call returns plaintext and writes an audit row, so a reveal query runs
 * only when the operator asks: never again because the window regained focus or the
 * network came back, and never kept in the cache once its last subscriber unmounts.
 */
export const REVEAL_QUERY_OPTIONS = {
  staleTime: 0,
  gcTime: 0,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;
