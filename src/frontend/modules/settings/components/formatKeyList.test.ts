// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { formatKeyList } from './formatKeyList';

describe('formatKeyList', () => {
  const keys = ['updates.manage', 'environments.manage'];

  it('uses the active language for the conjunction', () => {
    expect(formatKeyList(keys, 'en')).toBe('updates.manage and environments.manage');
    expect(formatKeyList(keys, 'de')).toBe('updates.manage und environments.manage');
    expect(formatKeyList(keys, 'nl')).toBe('updates.manage en environments.manage');
    expect(formatKeyList(keys, 'pt')).toBe('updates.manage e environments.manage');
  });

  it('leaves a single key bare', () => {
    expect(formatKeyList(['updates.manage'], 'de')).toBe('updates.manage');
  });
});
