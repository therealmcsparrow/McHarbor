// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { McHarborUpdateCard } from './McHarborUpdateCard';

const SENTENCE = 'You cannot go back to an older version afterwards.';

function latest(version: string) {
  server.use(
    http.get('*/api/about', () =>
      HttpResponse.json({ success: true, data: { version: '2026.11.01', native: false, sandboxed: false } }),
    ),
    http.get('*/api/updates/check', () =>
      HttpResponse.json({
        success: true,
        data: { currentVersion: '2026.11.01', latestVersion: version, updateAvailable: true },
      }),
    ),
  );
}

async function openConfirm() {
  const user = userEvent.setup();
  renderWithProviders(<McHarborUpdateCard />);
  await user.click(screen.getByRole('button', { name: 'Check Now' }));
  await user.click(await screen.findByTestId('self-update-button'));
  return within(await screen.findByRole('dialog'));
}

describe('McHarborUpdateCard manual prerelease install', () => {
  beforeEach(() => grantPermissions('*'));

  it('says there is no way back before a prerelease is installed by hand', async () => {
    latest('2026.11.02-rc.1');
    expect((await openConfirm()).getByText(new RegExp(SENTENCE))).toBeInTheDocument();
  });

  it('leaves the sentence out for a stable update', async () => {
    latest('2026.11.02');
    const dialog = await openConfirm();
    expect(dialog.queryByText(new RegExp(SENTENCE))).not.toBeInTheDocument();
  });
});

describe('McHarborUpdateCard prerelease labels', () => {
  beforeEach(() => grantPermissions('*'));

  function check(current: string, latest: string, update: boolean) {
    server.use(
      http.get('*/api/about', () =>
        HttpResponse.json({ success: true, data: { version: current, native: false, sandboxed: false } }),
      ),
      http.get('*/api/updates/channels', () =>
        HttpResponse.json({ success: true, data: { mcharborPrereleases: false } }),
      ),
      http.get('*/api/updates/check', () =>
        HttpResponse.json({
          success: true,
          data: { currentVersion: current, latestVersion: latest, updateAvailable: update },
        }),
      ),
    );
  }

  async function checkNow() {
    const user = userEvent.setup();
    renderWithProviders(<McHarborUpdateCard />);
    await user.click(screen.getByRole('button', { name: 'Check Now' }));
  }

  it('labels an offered prerelease as text next to the version', async () => {
    check('2026.11.01', '2026.11.02-rc.1', true);
    await checkNow();
    expect(await screen.findAllByText(/2026.11.02-rc.1/)).not.toHaveLength(0);
    expect(screen.getAllByText('Prerelease').length).toBeGreaterThan(0);
  });

  it('labels no stable version', async () => {
    check('2026.11.01', '2026.11.02', true);
    await checkNow();
    await screen.findByTestId('self-update-button');
    expect(screen.queryByText('Prerelease')).not.toBeInTheDocument();
  });

  it('says in this card that a prerelease is running while the switch is off', async () => {
    check('2026.11.02-rc.1', '2026.11.02-rc.1', false);
    await checkNow();
    expect(
      await screen.findByText(/You are running 2026\.11\.02-rc\.1 \(Prerelease\)\. Prerelease updates are off\./),
    ).toBeInTheDocument();
    expect(screen.getByText('Prerelease')).toBeInTheDocument();
  });
});

describe('McHarborUpdateCard native manual install', () => {
  beforeEach(() => grantPermissions('*'));

  it('names the offered prerelease version on a stable native host', async () => {
    const posts = vi.fn();
    server.use(
      http.get('*/api/about', () =>
        HttpResponse.json({
          success: true,
          data: { version: '2026.11.01', native: true, sandboxed: false, selfUpdate: true },
        }),
      ),
      http.get('*/api/updates/check', () =>
        HttpResponse.json({
          success: true,
          data: { currentVersion: '2026.11.01', latestVersion: '2026.11.02-rc.1', updateAvailable: true },
        }),
      ),
      http.get('*/api/updates/channels', () =>
        HttpResponse.json({ success: true, data: { mcharborPrereleases: true } }),
      ),
      http.get('*/api/health', () => HttpResponse.json({ success: true, data: { status: 'ok', instanceId: 'i1' } })),
      http.post('*/api/versions/self-update', async ({ request }) => {
        posts(await request.json());
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<McHarborUpdateCard />);
    await user.click(screen.getByRole('button', { name: 'Check Now' }));
    await user.click(await screen.findByTestId('self-update-button'));
    const dialog = within(await screen.findByRole('dialog'));
    await user.click(dialog.getByRole('button', { name: 'Update now' }));
    await waitFor(() => expect(posts).toHaveBeenCalledTimes(1));
    expect(posts).toHaveBeenCalledWith({ version: '2026.11.02-rc.1' });
  });
});
