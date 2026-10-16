// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { AppLogo } from './AppLogo';

describe('AppLogo', () => {
  it('renders the logo image when one is set', () => {
    render(<AppLogo appName="Grafana" logo="https://example.test/grafana.svg" />);
    expect(screen.getByRole('img', { name: 'Grafana' })).toHaveAttribute('src', 'https://example.test/grafana.svg');
  });

  it('falls back to the initial when the logo fails to load', () => {
    render(<AppLogo appName="grafana" logo="https://example.test/missing.svg" />);
    fireEvent.error(screen.getByRole('img', { name: 'grafana' }));
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByText('G')).toBeInTheDocument();
  });

  it('tries a new logo URL after an earlier one failed', () => {
    const { rerender } = render(<AppLogo appName="Grafana" logo="https://example.test/missing.svg" />);
    fireEvent.error(screen.getByRole('img', { name: 'Grafana' }));
    rerender(<AppLogo appName="Grafana" logo="https://example.test/fixed.svg" />);
    expect(screen.getByRole('img', { name: 'Grafana' })).toHaveAttribute('src', 'https://example.test/fixed.svg');
  });

  it('shows the initial when there is no logo at all', () => {
    render(<AppLogo appName="Adminer" logo="" />);
    expect(screen.getByText('A')).toBeInTheDocument();
  });
});
