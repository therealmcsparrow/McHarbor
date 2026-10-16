// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it, vi } from 'vitest';
import {
  formatBytes,
  formatDate,
  formatDateOnly,
  formatTime,
  formatUptime,
  splitBytes,
  timeAgo,
  truncateId,
} from './format';

vi.mock('@core/i18n/i18n', () => ({
  default: { language: 'en' },
}));

describe('formatBytes', () => {
  it('formats empty and scaled byte values', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1024)).toBe('1 KB');
    expect(formatBytes(1536, 2)).toBe('1.5 KB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5 MB');
  });
});

describe('splitBytes', () => {
  it('splits byte values into value and unit parts', () => {
    expect(splitBytes(0)).toEqual({ value: '0', unit: 'B' });
    expect(splitBytes(1536, 2)).toEqual({ value: '1.5', unit: 'KB' });
  });
});

describe('formatUptime', () => {
  it('formats minutes, hours, and days', () => {
    expect(formatUptime(0)).toBe('0m');
    expect(formatUptime(59)).toBe('0m');
    expect(formatUptime(90)).toBe('1m');
    expect(formatUptime(3660)).toBe('1h 1m');
    expect(formatUptime(90000)).toBe('1d 1h');
  });
});

describe('date formatting', () => {
  it('returns a dash for empty or invalid dates', () => {
    expect(formatDate(null)).toBe('-');
    expect(formatDateOnly(undefined)).toBe('-');
    expect(formatTime('not-a-date')).toBe('-');
  });
});

describe('timeAgo', () => {
  it('handles missing, invalid, future, recent, and older dates', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-28T12:00:00Z'));

    expect(timeAgo(null)).toBe('-');
    expect(timeAgo('not-a-date')).toBe('-');
    expect(timeAgo('2026-05-28T12:00:10Z')).toBe('now');
    expect(timeAgo('2026-05-28T11:59:30Z')).toBe('now');
    expect(timeAgo('2026-05-28T11:55:00Z')).toBe('5 minutes ago');
    expect(timeAgo('2026-05-28T10:00:00Z')).toBe('2 hours ago');
    expect(timeAgo('2026-05-26T12:00:00Z')).toBe('2 days ago');

    vi.useRealTimers();
  });
});

describe('truncateId', () => {
  it('truncates IDs and handles empty values', () => {
    expect(truncateId(null)).toBe('');
    expect(truncateId('abcdef1234567890')).toBe('abcdef123456');
    expect(truncateId('abcdef1234567890', 6)).toBe('abcdef');
  });
});
