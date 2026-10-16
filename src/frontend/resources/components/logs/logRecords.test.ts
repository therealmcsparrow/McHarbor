// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { createSnapshotLogRecords } from './logRecords';

describe('createSnapshotLogRecords timestamps', () => {
  it('splits a Docker ISO stamp off the message', () => {
    const [record] = createSnapshotLogRecords('2026-10-08T10:00:00.123456789Z listening on :80');
    expect(record?.timestamp).toBe('2026-10-08T10:00:00.123456789Z');
    expect(record?.message).toBe('listening on :80');
    expect(record?.text).toBe('2026-10-08T10:00:00.123456789Z listening on :80');
  });

  it('reads a two-part date and time as one local stamp', () => {
    const [record] = createSnapshotLogRecords('2026-10-08 10:00:00 ERROR vm/stop web');
    expect(record?.timestamp).toBe('2026-10-08T10:00:00');
    expect(new Date(record?.timestamp ?? '').getHours()).toBe(10);
    expect(record?.level).toBe('error');
  });

  it('turns a journal stamp into a parseable ISO time near now', () => {
    const now = new Date();
    const month = now.toLocaleString('en-US', { month: 'short' });
    const stamp = `${month} ${String(now.getDate()).padStart(2, ' ')} 00:00:01`;
    const [record] = createSnapshotLogRecords(`${stamp} host sshd[1]: ok`);
    expect(record?.message).toBe('host sshd[1]: ok');
    const parsed = Date.parse(record?.timestamp ?? '');
    expect(Number.isNaN(parsed)).toBe(false);
    expect(Math.abs(parsed - now.getTime())).toBeLessThan(86_400_000 * 2);
  });

  it('leaves a line without a wall-clock time unstamped', () => {
    const [record] = createSnapshotLogRecords('[    1.234] kernel: booted');
    expect(record?.timestamp).toBe('');
    expect(record?.message).toBe('[    1.234] kernel: booted');
  });
});
