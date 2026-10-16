// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Badge, badgeVariants } from './Badge';

describe('Badge', () => {
  it('renders children with the default variant', () => {
    render(<Badge>Default</Badge>);

    expect(screen.getByText('Default')).toHaveClass('bg-primary/10');
  });

  it('supports custom variants and classes', () => {
    render(
      <Badge variant="success" className="custom-badge">
        Running
      </Badge>,
    );

    expect(screen.getByText('Running')).toHaveClass('bg-success/15', 'custom-badge');
  });

  it('exposes variant classes for direct composition', () => {
    expect(badgeVariants({ variant: 'warning' })).toContain('bg-warning/15');
    expect(badgeVariants({ variant: 'outline' })).toContain('border-border');
  });
});
