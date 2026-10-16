// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { LogFilterBar, type LogFilterBarProps } from './LogFilterBar';

function renderBar(overrides: Partial<LogFilterBarProps> = {}) {
  const props: LogFilterBarProps = {
    filter: '',
    onFilterChange: vi.fn(),
    regexError: false,
    regex: false,
    onRegexChange: vi.fn(),
    level: 'all',
    onLevelChange: vi.fn(),
    timeRange: 'all',
    onTimeRangeChange: vi.fn(),
    newestFirst: true,
    onNewestFirstChange: vi.fn(),
    ...overrides,
  };
  render(<LogFilterBar {...props} />);
  return props;
}

describe('LogFilterBar', () => {
  it('offers neither Live nor Export for a static log', () => {
    renderBar();
    expect(screen.queryByRole('button', { name: 'Live' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Export' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Regex' })).toBeInTheDocument();
  });

  it('offers Live and Export when the caller supplies their handlers', async () => {
    const user = userEvent.setup();
    const onLiveChange = vi.fn();
    const onExport = vi.fn();
    renderBar({ live: false, onLiveChange, onExport });

    await user.click(screen.getByRole('button', { name: 'Live' }));
    await user.click(screen.getByRole('button', { name: 'Export' }));

    expect(onLiveChange).toHaveBeenCalledWith(true);
    expect(onExport).toHaveBeenCalledTimes(1);
  });

  it('explains an invalid regular expression on the filter field', () => {
    renderBar({ filter: '(', regex: true, regexError: true });
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Enter a valid regular expression.');
    expect(screen.getByRole('textbox', { name: 'Filter log records' })).toHaveAttribute('aria-describedby', alert.id);
  });
});
