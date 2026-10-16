// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { ct } from './canvas-theme';
import { CATEGORY_COLORS, CATEGORY_GLOW_CLASS, CATEGORY_TAG_COLORS } from '@resources/nodes/registry';

describe('workflow constants', () => {
  it('exports central canvas theme tokens', () => {
    expect(ct.text40).toBe('text-foreground/40');
    expect(ct.canvasBg).toBe('bg-background');
    expect(ct.tooltipBg).toBe('bg-popover/95');
  });

  it('exports workflow node category styles', () => {
    expect(CATEGORY_COLORS.trigger).toEqual({ header: 'bg-success', border: 'border-success/30' });
    expect(CATEGORY_TAG_COLORS.integration).toBe('bg-chart-3/20 text-chart-3');
    expect(CATEGORY_GLOW_CLASS.utility).toContain('ring-node-utility/40');
  });
});
