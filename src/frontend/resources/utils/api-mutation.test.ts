// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { assertSuccess } from './api-mutation';

describe('assertSuccess', () => {
  it('returns response data for successful API responses', () => {
    expect(assertSuccess({ success: true, data: { id: 'one' } })).toEqual({ id: 'one' });
  });

  it('throws API errors or a default failure message', () => {
    expect(() => assertSuccess({ success: false, error: 'Denied' })).toThrow('Denied');
    expect(() => assertSuccess({ success: false })).toThrow('Operation failed');
  });
});
