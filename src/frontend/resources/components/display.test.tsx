// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Sparkline } from './Sparkline';
import { StatCard } from './StatCard';

describe('StatCard', () => {
  it('renders optional icon, unit, description, trend, and chart', () => {
    render(
      <StatCard
        title="CPU"
        value={42}
        unit="%"
        description="Average"
        icon={<span data-testid="icon">I</span>}
        accentColor="rgb(1, 2, 3)"
        trend={{ value: 5, label: 'since last hour' }}
        chart={<span>Chart</span>}
        className="stat-extra"
      />,
    );

    expect(screen.getByText('CPU').closest('.stat-extra')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('%')).toHaveClass('text-primary');
    expect(screen.getByText('Average')).toBeInTheDocument();
    expect(screen.getByText('+5% since last hour')).toHaveClass('text-success');
    expect(screen.getByText('Chart')).toBeInTheDocument();
    expect(screen.getByTestId('icon')).toHaveStyle({ color: 'rgb(1, 2, 3)' });
  });

  it('renders negative trends and default icon color', () => {
    render(<StatCard title="Memory" value="1.2" icon={<span>Icon</span>} trend={{ value: -3, label: 'change' }} />);

    expect(screen.getByText('-3% change')).toHaveClass('text-danger');
    expect(screen.getByText('Icon').parentElement).toHaveClass('text-muted-foreground');
  });
});

describe('Sparkline', () => {
  it('renders calculated polyline points', () => {
    const { container } = render(
      <Sparkline data={[10, 20, 10]} width={20} height={10} color="red" className="spark-extra" />,
    );

    expect(container.querySelector('svg')).toHaveClass('spark-extra');
    expect(container.querySelector('polyline')).toHaveAttribute('points', '0,10 10,0 20,10');
    expect(container.querySelector('polyline')).toHaveAttribute('stroke', 'red');
  });

  it('renders nothing for fewer than two points', () => {
    const { container } = render(<Sparkline data={[1]} />);

    expect(container.firstChild).toBeNull();
  });
});
