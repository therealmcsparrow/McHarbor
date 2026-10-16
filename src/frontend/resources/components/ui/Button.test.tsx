// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Button, buttonVariants } from './Button';

describe('Button', () => {
  it('renders a clickable button with default styling', () => {
    const onClick = vi.fn();

    render(<Button onClick={onClick}>Save</Button>);
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(onClick).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', { name: 'Save' })).toHaveClass('bg-primary');
  });

  it('supports variants, sizes, disabled state, and custom classes', () => {
    render(
      <Button variant="destructive" size="sm" className="extra-class" disabled>
        Delete
      </Button>,
    );

    const button = screen.getByRole('button', { name: 'Delete' });
    expect(button).toBeDisabled();
    expect(button).toHaveClass('bg-danger/15', 'text-xs', 'extra-class');
  });

  it('renders through Slot when asChild is enabled', () => {
    render(
      <Button asChild variant="link">
        <a href="/docs">Docs</a>
      </Button>,
    );

    const link = screen.getByRole('link', { name: 'Docs' });
    expect(link).toHaveAttribute('href', '/docs');
    expect(link).toHaveClass('text-primary');
  });

  it('exposes variant classes for direct composition', () => {
    expect(buttonVariants({ variant: 'outline', size: 'icon' })).toContain('mc-glass');
    expect(buttonVariants({ variant: 'outline', size: 'icon' })).toContain('aspect-square');
  });
});
