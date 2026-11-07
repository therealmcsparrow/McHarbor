// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PrereleaseChannelsCard } from './PrereleaseChannelsCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const puts = vi.fn();
const reads = vi.fn();
let stored: Record<string, boolean>;
let state: Record<string, unknown>;
let updateSettings: Record<string, unknown>;

beforeEach(() => {
  puts.mockReset();
  reads.mockReset();
  stored = {
    mcharborPrereleases: false,
    agentPrereleases: false,
    pluginPrereleases: false,
    mcharborAutoPrereleases: false,
    agentAutoPrereleases: false,
  };
  state = { currentVersion: '2026.11.01', latestVersion: '2026.11.01', updateAvailable: false, intervalHours: 6 };
  updateSettings = { enabled: true, autoInstall: true, autoUpdateAgents: true, agentExcludedEnvIds: [] };
  grantPermissions(['updates.view', 'updates.manage']);
  server.use(
    http.get('*/api/updates/channels', () => {
      reads();
      return HttpResponse.json({ success: true, data: stored });
    }),
    http.put('*/api/updates/channels', async ({ request }) => {
      const body = (await request.json()) as Record<string, boolean>;
      puts(body);
      stored = { ...stored, ...body };
      return HttpResponse.json({ success: true, data: stored });
    }),
    http.get('*/api/updates/state', () => HttpResponse.json({ success: true, data: state })),
    http.get('*/api/updates/settings', () => HttpResponse.json({ success: true, data: updateSettings })),
  );
});

async function ready(name: string) {
  const toggle = await screen.findByRole('switch', { name });
  await waitFor(() => expect(toggle).not.toHaveAttribute('aria-busy', 'true'));
  return toggle;
}

describe('PrereleaseChannelsCard', () => {
  it('sends only the changed field when the McHarbor switch is turned on', async () => {
    const user = userEvent.setup();
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    const toggle = await ready('Include prereleases');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    await user.click(toggle);
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith({ mcharborPrereleases: true });
  });

  it('keeps the switch inert and names updates.manage as visible text when it is missing', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    const toggle = await ready('Include prereleases');
    expect(toggle).toHaveAttribute('aria-disabled', 'true');
    expect(toggle).toHaveAccessibleDescription(/Needs updates\.manage\./);
    await user.click(toggle);
    expect(puts).not.toHaveBeenCalled();
  });

  it('reads nothing and shows a notice without updates.view', async () => {
    grantPermissions([]);
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    expect(await screen.findByText('Needs updates.view to see these settings.')).toBeInTheDocument();
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    expect(reads).not.toHaveBeenCalled();
  });

  it('locks the automatic row until both switches it depends on are on, and says so', async () => {
    updateSettings = { ...updateSettings, autoInstall: false };
    stored.mcharborPrereleases = true;
    const user = userEvent.setup();
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    const auto = await ready('Also install prereleases automatically');
    await waitFor(() => expect(auto).toHaveAttribute('aria-disabled', 'true'));
    expect(auto).toHaveAccessibleDescription(/Needs Include prereleases and Install automatically\./);
    await user.click(auto);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(puts).not.toHaveBeenCalled();
  });

  it('asks before turning the automatic option on, and sends nothing on Cancel', async () => {
    stored.mcharborPrereleases = true;
    const user = userEvent.setup();
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    const auto = await ready('Also install prereleases automatically');
    await waitFor(() => expect(auto).not.toHaveAttribute('aria-disabled', 'true'));
    await user.click(auto);
    const dialog = await screen.findByRole('dialog', { name: 'Install prereleases automatically?' });
    expect(puts).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(puts).not.toHaveBeenCalled();
    await user.click(auto);
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Turn on' }));
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith({ mcharborAutoPrereleases: true });
  });

  it('leaves the McHarbor running text to the status region and the update card', async () => {
    state = { ...state, currentVersion: '2026.11.02-rc.1' };
    renderWithProviders(<PrereleaseChannelsCard scope="mcharbor" />);
    await ready('Include prereleases');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('says in text that a prerelease agent target is running while agent prereleases are off', async () => {
    state = { ...state, latestAgentVersion: '2026.11.02-rc.1' };
    renderWithProviders(<PrereleaseChannelsCard scope="agents" />);
    const status = await screen.findByRole('status');
    expect(status).toHaveTextContent('Target: 2026.11.02-rc.1 (Prerelease). Prerelease offers are off');
    expect(status).not.toHaveTextContent('downgrade');
  });

  it('shows a retry, never a blank card, when the settings cannot be read', async () => {
    server.use(
      http.get('*/api/updates/channels', () =>
        HttpResponse.json({ success: false, error: 'channels unavailable', code: 'err.internal' }, { status: 500 }),
      ),
    );
    renderWithProviders(<PrereleaseChannelsCard scope="agents" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load the prerelease settings.');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });

  it('keeps the agent switches independent of the McHarbor ones', async () => {
    const user = userEvent.setup();
    renderWithProviders(<PrereleaseChannelsCard scope="agents" />);
    await user.click(await ready('Include agent prereleases'));
    await waitFor(() => expect(puts).toHaveBeenCalledTimes(1));
    expect(puts).toHaveBeenCalledWith({ agentPrereleases: true });
  });
});
