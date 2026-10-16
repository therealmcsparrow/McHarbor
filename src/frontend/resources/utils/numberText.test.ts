// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { toNumberOrNull, toNumberText } from './numberText';

describe('numberText', () => {
  it('reads blank and non-numeric text as null and keeps real numbers', () => {
    expect(toNumberOrNull('')).toBeNull();
    expect(toNumberOrNull('   ')).toBeNull();
    expect(toNumberOrNull('abc')).toBeNull();
    expect(toNumberOrNull(undefined)).toBeNull();
    expect(toNumberOrNull(Number.NaN)).toBeNull();
    expect(toNumberOrNull('0')).toBe(0);
    expect(toNumberOrNull('-1.5')).toBe(-1.5);
    expect(toNumberOrNull(42)).toBe(42);
  });

  it('round-trips a value back to the text form state keeps', () => {
    expect(toNumberText(null)).toBe('');
    expect(toNumberText(0)).toBe('0');
    for (const text of ['0', '7', '1.25', '-3', '']) {
      expect(toNumberText(toNumberOrNull(text))).toBe(text);
    }
  });
});
