// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { extractLogTail } from './log-tail';

describe('extractLogTail', () => {
  it('returns an empty tail for missing log output', () => {
    expect(extractLogTail(undefined)).toEqual([]);
    expect(extractLogTail('')).toEqual([]);
  });

  it('strips ANSI escapes, trims trailing whitespace, drops empty lines, and limits from the end', () => {
    const raw = ['\u001B[32mfirst\u001B[0m   ', '', 'second', 'third', 'fourth'].join('\n');

    expect(extractLogTail(raw, 2)).toEqual(['third', 'fourth']);
    expect(extractLogTail(raw, 10)).toEqual(['first', 'second', 'third', 'fourth']);
  });
});
