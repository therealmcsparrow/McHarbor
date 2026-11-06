// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentReleaseSettingsCard } from './AgentReleaseSettingsCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const settings = {
  enabled: true,
  mode: 'notify',
  checkCron: '0 5 * * *',
  keep: 3,
  sources: [{ kind: 'mirror', url: 'https://mirror.example/agents' }],
  imageRepository: 'ghcr.io/therealmcsparrow/mcharbor-agent',
  imagePlatforms: ['linux/amd64'],
};

// A URL here passes for a repository until Docker refuses every pull with
// "invalid reference format", so the field has to name the mistake itself.
describe('AgentReleaseSettingsCard image repository', () => {
  const saves = vi.fn();

  beforeEach(() => {
    saves.mockClear();
    grantPermissions(['updates.view', 'updates.manage']);
    server.use(
      http.get('*/api/updates/agents/releases', () =>
        HttpResponse.json({ success: true, data: { target: '', state: {}, settings, releases: [] } }),
      ),
      http.put('*/api/updates/agents/releases/settings', async ({ request }) => {
        saves(await request.json());
        return HttpResponse.json({ success: true, data: settings });
      }),
    );
  });

  async function typeRepository(value: string) {
    const user = userEvent.setup();
    renderWithProviders(<AgentReleaseSettingsCard />);
    const field = await screen.findByRole('textbox', { name: 'Image repository' });
    await user.clear(field);
    await user.type(field, value);
    return user;
  }

  it('names the mistake and blocks saving when a URL is entered', async () => {
    const user = await typeRepository('https://www.mcharbor.eu/catalog/agents');
    expect(screen.getByRole('alert')).toHaveTextContent('not a URL');
    const save = screen.getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.click(save);
    expect(saves).not.toHaveBeenCalled();
  });

  it('saves a registry with a port, which also contains a colon', async () => {
    const user = await typeRepository('registry.example:5000/agent');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves).toHaveBeenCalledWith(expect.objectContaining({ imageRepository: 'registry.example:5000/agent' }));
  });
});
