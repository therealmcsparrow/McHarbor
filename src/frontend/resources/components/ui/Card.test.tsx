// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './Card';

describe('Card', () => {
  it('renders the full card structure with forwarded classes', () => {
    render(
      <Card className="card-extra" data-testid="card">
        <CardHeader className="header-extra">
          <CardTitle>Title</CardTitle>
          <CardDescription>Description</CardDescription>
        </CardHeader>
        <CardContent>Content</CardContent>
        <CardFooter>Footer</CardFooter>
      </Card>,
    );

    expect(screen.getByTestId('card')).toHaveClass('mc-glass', 'card-extra');
    expect(screen.getByText('Title')).toHaveClass('font-semibold');
    expect(screen.getByText('Description')).toHaveClass('text-muted-foreground');
    expect(screen.getByText('Content')).toHaveClass('p-4');
    expect(screen.getByText('Footer')).toHaveClass('border-t');
  });
});
