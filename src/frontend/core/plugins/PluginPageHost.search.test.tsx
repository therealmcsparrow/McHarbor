// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../test/render';
import { PluginPageHost } from './PluginPageHost';
import type { PluginNavContext } from './PluginPageHost';

vi.mock('./loader', () => ({
  usePluginInstalled: () => [{ slug: 'fictional-plugin', loaded: true }],
}));

const GLOBAL = 'McharborPlugin_fictional_plugin';

function Jump({ to }: { to: string }) {
  const navigate = useNavigate();
  return <button onClick={() => navigate(to)}>jump</button>;
}

function renderAt(entry: string, jumpTo = '/plugins/fictional-plugin?action=import') {
  return renderWithProviders(
    <MemoryRouter initialEntries={[entry]}>
      <Jump to={jumpTo} />
      <Routes>
        <Route path="/plugins/:slug/*" element={<PluginPageHost />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('plugin page host hands the query string to the plugin', () => {
  const mount = vi.fn<(el: HTMLElement, ctx?: PluginNavContext) => void>();
  const onNavigate = vi.fn<(ctx: PluginNavContext) => void>();

  beforeEach(() => {
    mount.mockReset();
    onNavigate.mockReset();
    (window as unknown as Record<string, unknown>)[GLOBAL] = { mount, onNavigate };
  });

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>)[GLOBAL];
  });

  it('mounts with the palette action in ctx.search, without the leading question mark', () => {
    renderAt('/plugins/fictional-plugin?action=create');
    expect(mount).toHaveBeenCalledTimes(1);
    const ctx = mount.mock.calls.at(0)?.[1] as PluginNavContext;
    expect(ctx.search).toBe('action=create');
    expect(new URLSearchParams(ctx.search).get('action')).toBe('create');
    expect(ctx.slug).toBe('fictional-plugin');
  });

  it('keeps the other query parameters next to the action', () => {
    renderAt('/plugins/fictional-plugin/vms?tab=disks&action=import');
    const ctx = mount.mock.calls.at(0)?.[1] as PluginNavContext;
    expect(ctx.splat).toBe('vms');
    expect(new URLSearchParams(ctx.search).get('action')).toBe('import');
    expect(new URLSearchParams(ctx.search).get('tab')).toBe('disks');
  });

  it('tells a mounted plugin about a later action without remounting it', async () => {
    renderAt('/plugins/fictional-plugin');
    expect(mount.mock.calls.at(0)?.[1]?.search).toBe('');
    await act(async () => {
      await userEvent.click(screen.getByRole('button', { name: 'jump' }));
    });
    expect(mount).toHaveBeenCalledTimes(1);
    const last = onNavigate.mock.calls.at(-1)?.[0];
    expect(last?.search).toBe('action=import');
  });
});
