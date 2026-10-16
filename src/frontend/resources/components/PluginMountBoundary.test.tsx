// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginMountBoundary } from '@resources/components/PluginMountBoundary';

let shouldThrow = true;

function Probe() {
  if (shouldThrow) throw new Error('mount exploded');
  return <p>plugin view</p>;
}

describe('PluginMountBoundary', () => {
  beforeEach(() => {
    shouldThrow = true;
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
  });
  afterEach(() => vi.restoreAllMocks());

  it('renders the children when nothing throws', () => {
    shouldThrow = false;
    render(
      <PluginMountBoundary>
        <Probe />
      </PluginMountBoundary>,
    );
    expect(screen.getByText('plugin view')).toBeInTheDocument();
  });

  it('shows the failure state with the thrown message instead of blanking', () => {
    render(
      <PluginMountBoundary>
        <Probe />
      </PluginMountBoundary>,
    );
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('mount exploded')).toBeInTheDocument();
    expect(screen.queryByText('plugin view')).not.toBeInTheDocument();
  });

  it('retry remounts the subtree', async () => {
    render(
      <PluginMountBoundary>
        <Probe />
      </PluginMountBoundary>,
    );
    shouldThrow = false;
    await userEvent.click(screen.getByRole('button', { name: /try again/i }));
    expect(screen.getByText('plugin view')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
