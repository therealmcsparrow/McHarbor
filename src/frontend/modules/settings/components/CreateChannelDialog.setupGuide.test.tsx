// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it, vi } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { CreateChannelDialog } from './CreateChannelDialog';

vi.mock('@core/plugins/PluginFormOutlet', () => ({
  PluginFormOutlet: ({ kind }: { kind: string }) => <div data-testid={`outlet-${kind}`} />,
}));

const ADAPTERS = {
  smtp: {
    ChannelType: 'smtp',
    DisplayName: 'Mail relay',
    Capabilities: { EncryptedCredentials: true, SupportsRichAttachments: false },
    ConfigSchema: [],
  },
};

async function openConfigStep() {
  server.use(
    http.get('*/api/plugins/loader/notification-adapters', () => HttpResponse.json({ success: true, data: ADAPTERS })),
  );
  renderWithProviders(<CreateChannelDialog open onOpenChange={() => undefined} />);
  await userEvent.click(await screen.findByRole('button', { name: /Mail relay/ }));
}

describe('CreateChannelDialog setup guide', () => {
  it('closes the guide when Back returns to the type selector', async () => {
    await openConfigStep();
    await userEvent.click(await screen.findByRole('button', { name: 'Setup guide' }));
    expect(screen.getByTestId('outlet-setup_guide')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Back' }));

    await waitFor(() => expect(screen.getByRole('button', { name: /Mail relay/ })).toBeInTheDocument());
    expect(screen.queryByTestId('outlet-setup_guide')).not.toBeInTheDocument();
  });

  it('re-enters the config step with the guide closed', async () => {
    await openConfigStep();
    await userEvent.click(await screen.findByRole('button', { name: 'Setup guide' }));
    await userEvent.click(screen.getByRole('button', { name: 'Back' }));
    await userEvent.click(await screen.findByRole('button', { name: /Mail relay/ }));

    expect(await screen.findByRole('button', { name: 'Setup guide' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByTestId('outlet-setup_guide')).not.toBeInTheDocument();
  });

  it('wraps the guide in a bordered panel apart from the form', async () => {
    await openConfigStep();
    await userEvent.click(await screen.findByRole('button', { name: 'Setup guide' }));

    const panel = screen.getByTestId('outlet-setup_guide').parentElement;
    expect(panel).toHaveClass('mt-4', 'border', 'p-4');
  });
});
