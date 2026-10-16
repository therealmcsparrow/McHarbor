// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { cn } from './cn';

describe('cn', () => {
  it('joins conditional classes and resolves Tailwind conflicts', () => {
    expect(cn('px-2', { hidden: false }, ['text-sm', 'px-4'])).toBe('text-sm px-4');
  });
});
