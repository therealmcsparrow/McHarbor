// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { normalizeProcesses } from './normalizeProcesses';

describe('normalizeProcesses', () => {
  it('turns a null process list and null titles into empty arrays', () => {
    expect(normalizeProcesses({ Titles: null, Processes: null })).toEqual({ Titles: [], Processes: [] });
  });

  it('turns a null row into an empty row', () => {
    expect(normalizeProcesses({ Titles: ['PID'], Processes: [null, ['1']] })).toEqual({
      Titles: ['PID'],
      Processes: [[], ['1']],
    });
  });

  it('passes a populated response through unchanged', () => {
    const data = { Titles: ['UID', 'PID'], Processes: [['root', '1']] };
    expect(normalizeProcesses(data)).toEqual(data);
  });
});
