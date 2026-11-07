// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PluginCard } from './PluginCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { PluginEntry } from '../types/plugins';

beforeEach(() => {
  grantPermissions(['updates.view']);
  server.use(
    http.get('*/api/versions', () =>
      HttpResponse.json({
        success: true,
        data: { mcharbor: { name: 'McHarbor', version: '2026.11.02-rc.1' }, agents: [] },
      }),
    ),
  );
});

const plugin: PluginEntry & { installed: boolean } = {
  installed: true,
  slug: 'demo-plugin',
  name: 'Demo plugin',
  version: '2026.11.01',
  author: 'Demo',
  license: 'MIT',
  homepage: '',
  description: '',
  category: 'tools',
  download_url: '',
  versions: [],
};

function card(slug: string, installedVersion: string) {
  return (
    <PluginCard
      plugin={{ ...plugin, slug, name: slug }}
      installedVersion={installedVersion}
      onInstall={vi.fn()}
      onUninstall={vi.fn()}
      onViewDetail={vi.fn()}
    />
  );
}

describe('PluginCard host note', () => {
  it('names the prerelease host for a plugin that is itself on a prerelease', async () => {
    renderWithProviders(card('on-pre', '2026.11.02-rc.1'));
    expect(await screen.findByText(/Built for this prerelease host/)).toBeInTheDocument();
  });

  it('stays silent for a plugin that fell back to a stable build on a prerelease host', async () => {
    renderWithProviders(
      <>
        {card('on-pre', '2026.11.02-rc.1')}
        {card('on-stable', '2026.11.01')}
      </>,
    );
    // The prerelease sibling shares the host query: once its note shows, the host version has arrived.
    await screen.findByText(/Built for this prerelease host/);
    expect(screen.getAllByText(/Built for this prerelease host/)).toHaveLength(1);
  });
});
