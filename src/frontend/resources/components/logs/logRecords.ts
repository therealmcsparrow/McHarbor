// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { LogStreamRecord } from '@resources/hooks/useContainerLogs';
import { detectLogLevel } from '@resources/utils/log-severity';

export { detectLogLevel };

export type LogDisplayRecord = LogStreamRecord & {
  level: string;
  text: string;
};

const ISO_LEADING = /^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?/;
const SYSLOG_LEADING = /^([A-Z][a-z]{2})\s+(\d{1,2})\s+(\d{2}:\d{2}:\d{2})(?!\d)/;
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

function syslogToIso(month: string, day: string, time: string): string {
  const index = MONTHS.indexOf(month);
  if (index < 0) return '';
  const now = new Date();
  const [hours = 0, minutes = 0, seconds = 0] = time.split(':').map(Number);
  let candidate = new Date(now.getFullYear(), index, Number(day), hours, minutes, seconds);
  // Syslog stamps carry no year, so a stamp more than a day ahead belongs to last year.
  if (candidate.getTime() - now.getTime() > 86_400_000) {
    candidate = new Date(now.getFullYear() - 1, index, Number(day), hours, minutes, seconds);
  }
  return Number.isNaN(candidate.getTime()) ? '' : candidate.toISOString();
}

/** Splits a leading timestamp (Docker ISO, `YYYY-MM-DD HH:MM:SS` or journal/syslog `Mon DD HH:MM:SS`) off a line. */
function splitTimestamp(line: string): { timestamp: string; message: string } {
  const iso = ISO_LEADING.exec(line);
  if (iso) {
    const stamp = iso[0];
    return {
      timestamp: stamp.replace(' ', 'T'),
      message: line.slice(stamp.length).trimStart(),
    };
  }
  const syslog = SYSLOG_LEADING.exec(line);
  if (syslog) {
    const timestamp = syslogToIso(syslog[1] ?? '', syslog[2] ?? '', syslog[3] ?? '');
    if (timestamp) {
      return {
        timestamp,
        message: line.slice(syslog[0].length).trimStart(),
      };
    }
  }
  const [first = '', ...rest] = line.split(' ');
  if (!Number.isNaN(Date.parse(first))) {
    return { timestamp: first, message: rest.join(' ') };
  }
  return { timestamp: '', message: line };
}

function hashLine(line: string) {
  let hash = 0;
  for (let index = 0; index < line.length; index++) {
    hash = (hash * 31 + line.charCodeAt(index)) | 0;
  }
  return Math.abs(hash).toString(36);
}

/** Turns plain text log output into display records, keeping the newest 1000 lines. */
export function createSnapshotLogRecords(data: string): LogDisplayRecord[] {
  const content = data.replace(/\n+$/, '');
  if (!content) return [];
  const occurrences = new Map<string, number>();
  return content
    .split('\n')
    .slice(-1000)
    .map((line) => {
      const lineHash = hashLine(line);
      const occurrence = (occurrences.get(lineHash) ?? 0) + 1;
      occurrences.set(lineHash, occurrence);
      const { timestamp, message } = splitTimestamp(line);
      return {
        id: `snapshot-${lineHash}-${occurrence}`,
        timestamp,
        stream: 'stdout',
        message,
        truncated: false,
        level: detectLogLevel(message),
        text: line,
      };
    });
}

/** Offers lines as a `.log` download so an export matches exactly what the filters left on screen. */
export function downloadLogLines(lines: string[], filename: string) {
  const blob = new Blob([lines.join('\n')], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}
