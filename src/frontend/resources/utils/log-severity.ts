// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type LogSeverity = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'unknown';

export const LOG_SEVERITY_CLASS: Record<string, string> = {
  trace: 'text-muted-foreground',
  debug: 'text-muted-foreground',
  info: 'text-info',
  warn: 'text-warning',
  error: 'text-danger',
  unknown: '',
};

export function detectLogLevel(message: string): LogSeverity {
  const match = message.match(/\b(trace|debug|info|warn(?:ing)?|error|fatal|panic)\b/i)?.[1]?.toLowerCase();
  if (match === 'warning') return 'warn';
  if (match === 'fatal' || match === 'panic') return 'error';
  return (match as LogSeverity) ?? 'unknown';
}

export function logSeverityClass(level: string | undefined) {
  return LOG_SEVERITY_CLASS[level ?? 'unknown'] ?? '';
}
