// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { UPDATE_CHANNELS_KEY } from '@resources/hooks/useUpdateChannels';
import { PluginHostNote } from './PluginHostNote';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

let hostVersion = '2026.11.02-rc.1';
let globalOn = false;

beforeEach(() => {
  hostVersion = '2026.11.02-rc.1';
  globalOn = false;
  grantPermissions(['updates.view']);
  server.use(
    http.get('*/api/versions', () =>
      HttpResponse.json({ success: true, data: { mcharbor: { name: 'McHarbor', version: hostVersion }, agents: [] } }),
    ),
  );
});

function render(override: boolean | undefined) {
  const view = renderWithProviders(<PluginHostNote override={override} />);
  view.client.setQueryData(UPDATE_CHANNELS_KEY, { pluginPrereleases: globalOn });
  return view;
}

// Absence only means something once the host version has arrived, so each silent case waits for it.
async function expectSilent(view: ReturnType<typeof render>) {
  await waitFor(() => expect(view.client.getQueryData(['versions'])).toBeDefined());
  await act(async () => {});
  expect(screen.queryByText(/Built for this prerelease host/)).not.toBeInTheDocument();
}

describe('PluginHostNote', () => {
  it('tells the operator the plugin was built for this prerelease host', async () => {
    render(undefined);
    expect(await screen.findByText(/Built for this prerelease host/)).toBeInTheDocument();
  });

  it('says nothing on a stable host', async () => {
    hostVersion = '2026.11.01';
    await expectSilent(render(undefined));
  });

  it('says nothing when the plugin has its own prerelease override on', async () => {
    await expectSilent(render(true));
  });

  it('says nothing when the Plugin Store switch is on', async () => {
    globalOn = true;
    await expectSilent(render(undefined));
  });
});
