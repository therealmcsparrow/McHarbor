// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './Table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from './Tabs';

describe('table UI primitives', () => {
  it('renders a table structure with forwarded classes', () => {
    render(
      <Table className="table-extra">
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>web</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(screen.getByRole('table')).toHaveClass('table-extra');
    expect(screen.getByRole('columnheader', { name: 'Name' })).toHaveClass('uppercase');
    expect(screen.getByRole('cell', { name: 'web' })).toHaveClass('text-foreground');
  });
});

describe('tabs UI primitives', () => {
  it('renders tabs with active content', () => {
    render(
      <Tabs defaultValue="one">
        <TabsList className="tabs-extra">
          <TabsTrigger value="one">One</TabsTrigger>
          <TabsTrigger value="two">Two</TabsTrigger>
        </TabsList>
        <TabsContent value="one">First panel</TabsContent>
        <TabsContent value="two">Second panel</TabsContent>
      </Tabs>,
    );

    expect(screen.getByRole('tablist')).toHaveClass('tabs-extra');
    expect(screen.getByRole('tab', { name: 'One' })).toHaveAttribute('data-state', 'active');
    expect(screen.getByText('First panel')).toBeInTheDocument();
  });
});
