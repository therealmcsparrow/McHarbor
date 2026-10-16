// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentReleasePullPicker } from './AgentReleasePullPicker';
import type { AgentRelease } from '../hooks/useAgentReleases';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const stored = (version: string, status: AgentRelease['status'], build = '') =>
  ({ version, status, build }) as AgentRelease;

const available = [
  { version: '2026.10.01', build: '202610060104' },
  { version: '2026.10.01', build: '202610052009' },
  { version: '2026.10.01', build: '202610050818' },
  { version: '2026.09.02', build: '202609281200' },
  { version: '2026.09.01', build: '202609181858' },
  { version: '2026.08.03' },
];

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Agent version to pull' }));
}

describe('AgentReleasePullPicker', () => {
  beforeEach(() => grantPermissions(['updates.view', 'updates.manage']));

  // The reported gap: every build was 2026.10.01, the latest, so only older
  // versions were ever offered. Older builds of the latest version must be.
  it('offers older builds of the latest version, but not its newest build or the stored one', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[stored('2026.09.02', 'ready', '202609281200'), stored('2026.08.03', 'failed')]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={vi.fn()}
      />,
    );
    await openPicker(user);
    expect(screen.getByRole('button', { name: /202610052009/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /202610050818/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /2026.09.01/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /2026.08.03/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /202610060104/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /202609281200/ })).not.toBeInTheDocument();
  });

  it('offers a newer build of a stored version, and hides only the exact stored build', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[stored('2026.10.01', 'ready', '202610050818')]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={vi.fn()}
      />,
    );
    await openPicker(user);
    expect(screen.getByRole('button', { name: /202610052009/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /202610050818/ })).not.toBeInTheDocument();
  });

  it('offers no build of a version that is being pulled', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[stored('2026.10.01', 'pulling')]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={vi.fn()}
      />,
    );
    await openPicker(user);
    expect(screen.queryByRole('button', { name: /202610052009/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /2026.09.01/ })).toBeInTheDocument();
  });

  it('pulls exactly the chosen build', async () => {
    const user = userEvent.setup();
    const onPull = vi.fn();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={onPull}
      />,
    );
    const pull = screen.getByRole('button', { name: 'Pull version' });
    expect(pull).toBeDisabled();
    await openPicker(user);
    await user.click(screen.getByRole('button', { name: /202610050818/ }));
    await user.click(pull);
    expect(onPull).toHaveBeenCalledTimes(1);
    expect(onPull).toHaveBeenCalledWith('2026.10.01', '202610050818');
  });

  it('pulls a version that lists no build without one', async () => {
    const user = userEvent.setup();
    const onPull = vi.fn();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={onPull}
      />,
    );
    await openPicker(user);
    await user.click(screen.getByRole('button', { name: /2026.08.03/ }));
    await user.click(screen.getByRole('button', { name: 'Pull version' }));
    expect(onPull).toHaveBeenCalledWith('2026.08.03', undefined);
  });

  it('is disabled and names updates.manage when the permission is missing', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    const onPull = vi.fn();
    renderWithProviders(
      <AgentReleasePullPicker
        available={available}
        releases={[]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={onPull}
      />,
    );
    await openPicker(user);
    await user.click(screen.getByRole('button', { name: /202610050818/ }));
    const pull = screen.getByRole('button', { name: /Pull version/ });
    expect(pull).toBeDisabled();
    await user.hover(pull.parentElement ?? pull);
    expect((await screen.findAllByText(/updates.manage/)).length).toBeGreaterThan(0);
    expect(onPull).not.toHaveBeenCalled();
  });

  it('renders nothing when only the newest build of the latest version is listed', () => {
    const { container } = renderWithProviders(
      <AgentReleasePullPicker
        available={[{ version: '2026.10.01', build: '202610060104' }]}
        releases={[]}
        latest="2026.10.01"
        latestBuild="202610060104"
        pending={false}
        onPull={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
