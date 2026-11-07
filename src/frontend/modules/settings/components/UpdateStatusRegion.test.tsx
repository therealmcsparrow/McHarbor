// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { QueryClient } from '@tanstack/react-query';
import { screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { updatesKeys } from '../hooks/queryKeys';
import { UPDATE_CHANNELS_KEY } from '@resources/hooks/useUpdateChannels';
import { UpdateStatusRegion } from './UpdateStatusRegion';

let current = '2026.11.02-rc.1';
let prereleasesOn = false;

beforeEach(() => {
  current = '2026.11.02-rc.1';
  prereleasesOn = false;
  grantPermissions(['updates.view']);
  server.use(
    http.get('*/api/updates/state', () =>
      HttpResponse.json({ success: true, data: { currentVersion: current, updateAvailable: false } }),
    ),
    http.get('*/api/updates/channels', () =>
      HttpResponse.json({ success: true, data: { mcharborPrereleases: prereleasesOn } }),
    ),
  );
});

async function bothLoaded(client: QueryClient) {
  await waitFor(() => {
    expect(client.getQueryState(UPDATE_CHANNELS_KEY)?.status).toBe('success');
    expect(client.getQueryState(updatesKeys.self)?.status).toBe('success');
  });
}

describe('UpdateStatusRegion', () => {
  it('is a labelled region that says the host runs a prerelease with offers off, and offers no way back', async () => {
    renderWithProviders(<UpdateStatusRegion />);
    const region = await screen.findByRole('region', { name: 'Update status' });
    expect(region).toHaveTextContent('You are running 2026.11.02-rc.1 (Prerelease). Prerelease updates are off.');
    expect(region).toHaveTextContent('No older version can be installed.');
  });

  it('is absent on a stable host', async () => {
    current = '2026.11.01';
    const { client } = renderWithProviders(<UpdateStatusRegion />);
    await bothLoaded(client);
    expect(screen.queryByRole('region', { name: 'Update status' })).not.toBeInTheDocument();
  });

  it('is absent when prerelease updates are on', async () => {
    prereleasesOn = true;
    const { client } = renderWithProviders(<UpdateStatusRegion />);
    await bothLoaded(client);
    expect(screen.queryByRole('region', { name: 'Update status' })).not.toBeInTheDocument();
  });
});
