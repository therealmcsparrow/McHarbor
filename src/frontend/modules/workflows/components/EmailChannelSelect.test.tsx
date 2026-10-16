// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PluginCapability } from '@core/plugins/loader';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { EmailChannelSelect } from './EmailChannelSelect';

const pigeonMail: PluginCapability = { plugin: 'pigeon-plugin', kind: 'email-channel', channelType: 'pigeon' };

describe('EmailChannelSelect', () => {
  const listed = vi.fn();

  beforeEach(() => {
    listed.mockReset();
    server.use(
      http.get('*/api/communication-channels', () => {
        listed();
        return HttpResponse.json({
          success: true,
          data: [
            { id: 'mail-1', name: 'Ops mail', channelType: 'pigeon', isDefault: false, enabled: true },
            { id: 'mail-2', name: 'Old mail', channelType: 'pigeon', isDefault: false, enabled: false },
            { id: 'chat-1', name: 'Ops chat', channelType: 'chat', isDefault: true, enabled: true },
          ],
        });
      }),
    );
  });
  afterEach(() => {
    clearPluginCapabilities();
    resetPermissions();
  });

  it('offers only the enabled channels whose plugin declares it can send email', async () => {
    givePluginCapabilities([pigeonMail]);
    grantPermissions(['communications.view']);
    const onChange = vi.fn();
    renderWithProviders(<EmailChannelSelect value="" onChange={onChange} />);

    await userEvent.click(screen.getByRole('button', { name: 'Select communication channel' }));
    const offered = await screen.findByText('Ops mail');
    expect(screen.queryByText('Ops chat')).not.toBeInTheDocument();
    expect(screen.queryByText('Old mail')).not.toBeInTheDocument();
    await userEvent.click(offered);

    expect(onChange).toHaveBeenCalledWith('mail-1');
  });

  it('still names the channel a stored workflow points at', async () => {
    givePluginCapabilities([pigeonMail]);
    grantPermissions(['communications.view']);
    renderWithProviders(<EmailChannelSelect value="mail-1" onChange={vi.fn()} />);

    expect(await screen.findByText('Ops mail')).toBeInTheDocument();
  });

  it('is disabled, saying why, and asks nothing when no email plugin is loaded', async () => {
    givePluginCapabilities([pigeonMail], ['pigeon-plugin']);
    grantPermissions(['communications.view']);
    renderWithProviders(<EmailChannelSelect value="" onChange={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Select communication channel' })).toBeDisabled();
    expect(screen.getByText('No installed communication plugin can send email.')).toBeInTheDocument();
    await waitFor(() => expect(listed).not.toHaveBeenCalled());
  });
});
