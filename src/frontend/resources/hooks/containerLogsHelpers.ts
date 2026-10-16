// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const MAX_LIVE_RECORDS = 1000;
const MAX_LIVE_BYTES = 2 * 1024 * 1024;
const MAX_RECONNECT_DELAY = 30_000;

export type LogConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'ended' | 'error';

export type LogStreamRecord = {
  id: string;
  timestamp: string;
  stream: 'stdout' | 'stderr';
  message: string;
  truncated: boolean;
};

export function isLogStreamRecord(value: unknown): value is LogStreamRecord {
  if (!value || typeof value !== 'object') return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.id === 'string' &&
    typeof record.timestamp === 'string' &&
    (record.stream === 'stdout' || record.stream === 'stderr') &&
    typeof record.message === 'string' &&
    typeof record.truncated === 'boolean'
  );
}

export function retainLiveRecords(records: LogStreamRecord[]) {
  let bytes = 0;
  let start = records.length;
  while (start > 0 && records.length - start < MAX_LIVE_RECORDS) {
    const record = records[start - 1];
    if (!record) break;
    const recordBytes = (record.message.length + record.timestamp.length + record.id.length) * 2;
    if (bytes + recordBytes > MAX_LIVE_BYTES) break;
    bytes += recordBytes;
    start--;
  }
  return records.slice(start);
}

export const RETRYABLE_END_REASONS = ['eof', 'session_limit', 'record_limit', 'byte_limit', 'failed'];

/** Reads the `reason` of a stream "end" event, defaulting to a clean eof when the payload is unusable. */
export function parseEndReason(data: string): string {
  try {
    const payload: unknown = JSON.parse(data);
    if (payload && typeof payload === 'object') {
      const reasonValue = (payload as Record<string, unknown>).reason;
      if (typeof reasonValue === 'string') return reasonValue;
    }
  } catch {
    return 'eof';
  }
  return 'eof';
}

export function reconnectBackoff(attempt: number): number {
  return Math.min(1000 * 2 ** (attempt - 1), MAX_RECONNECT_DELAY);
}
