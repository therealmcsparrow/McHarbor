// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { releaseLabel } from './releaseLabel';

describe('releaseLabel', () => {
  it('tells two builds of one version apart', () => {
    expect(releaseLabel('2026.09.01', '202610220131')).toBe('2026.09.01+202610220131');
    expect(releaseLabel('2026.09.01', '202610220131')).not.toBe(releaseLabel('2026.09.01', '202609181858'));
  });

  it('falls back to the bare version when the build is missing or unstamped', () => {
    expect(releaseLabel('2026.09.01')).toBe('2026.09.01');
    expect(releaseLabel('2026.09.01', '')).toBe('2026.09.01');
    expect(releaseLabel('2026.09.01', 'dev')).toBe('2026.09.01');
  });
});
