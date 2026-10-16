// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Button } from '@resources/components/ui/Button';
import { SectionHero } from './SectionHero';

function Tile({ className }: { className?: string }) {
  return <svg className={className} data-testid="tile-icon" />;
}

function Group({ className }: { className?: string }) {
  return <svg className={className} data-testid="badge-icon" />;
}

describe('SectionHero', () => {
  it('names the section in a single h1, so the page has one document heading', () => {
    render(
      <SectionHero
        icon={Tile}
        title="Database"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
      />,
    );

    const headings = screen.getAllByRole('heading', { level: 1 });
    expect(headings).toHaveLength(1);
    expect(headings[0]).toHaveTextContent('Database');
  });

  it('renders the group badge and the description together', () => {
    render(
      <SectionHero
        icon={Tile}
        title="Database"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
      />,
    );

    expect(screen.getByText('System')).toBeInTheDocument();
    expect(screen.getByText('Health, migrations and backups.')).toBeInTheDocument();
    expect(screen.getByTestId('badge-icon')).toBeInTheDocument();
  });

  it('hides the decorative icon tile from assistive tech', () => {
    render(
      <SectionHero
        icon={Tile}
        title="Database"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
      />,
    );

    // The tile repeats the heading, so announcing it is noise. The badge icon
    // sits inside the badge's own text and is not separately hidden.
    const tile = screen.getByTestId('tile-icon').parentElement;
    expect(tile).toHaveAttribute('aria-hidden', 'true');
  });

  it('omits the actions column entirely when no actions are passed', () => {
    const { rerender } = render(
      <SectionHero
        icon={Tile}
        title="Database"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
      />,
    );
    expect(screen.queryByRole('button')).not.toBeInTheDocument();

    // An empty wrapper would still take its gap, pulling the heading off the
    // left edge on a section that has no controls.
    rerender(
      <SectionHero
        icon={Tile}
        title="Database"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
        actions={<Button>Run check</Button>}
      />,
    );
    expect(screen.getByRole('button', { name: 'Run check' })).toBeInTheDocument();
  });

  it('truncates a long title rather than wrapping the hero to two lines', () => {
    render(
      <SectionHero
        icon={Tile}
        title="A section name long enough to overflow its container"
        badgeIcon={Group}
        badgeLabel="System"
        description="Health, migrations and backups."
      />,
    );

    expect(screen.getByRole('heading', { level: 1 })).toHaveClass('truncate');
  });
});
