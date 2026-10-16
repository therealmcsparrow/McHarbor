// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { TicketsPanel } from './TicketsPanel';
import { ChannelWebhookPanel } from './ChannelWebhookPanel';
import type { ItsmChannel, TicketLink } from '../hooks/useItsm';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const parked: TicketLink = {
  id: 'l1',
  channelId: 'ch1',
  channelType: 'servicenow',
  channelName: 'ServiceNow prod',
  dedupKey: 'a:e:s',
  alertId: 'a',
  alertName: 'CPU high',
  environmentId: 'e',
  subjectId: 's',
  externalId: 'sys-1',
  externalNumber: 'INC0012345',
  externalUrl: 'https://sn.example/INC0012345',
  localState: 'open',
  remoteState: 'acknowledged',
  remoteAssignee: 'jdoe',
  lastError: 'Assignment group "Ops" does not exist',
  openedAt: '',
  resolvedAt: '',
  lastSyncedAt: '2026-09-26T10:00:00Z',
  updatedAt: '2026-09-26T10:00:00Z',
  parked: true,
};

function row() {
  return screen.getByText('INC0012345').closest('tr') as HTMLElement;
}

describe('Tickets', () => {
  const retries = vi.fn();
  beforeEach(() => {
    retries.mockReset();
    grantPermissions(['alerts.view', 'alerts.manage']);
    server.use(
      http.get('*/api/itsm/links', () =>
        HttpResponse.json({
          success: true,
          data: {
            items: [parked],
            total: 1,
            page: 1,
            per_page: 200,
            total_pages: 1,
          },
        }),
      ),
      http.post('*/api/itsm/links/:id/retry', ({ params }) => {
        retries(params.id);
        return HttpResponse.json({ success: true, data: { requeued: 1 } });
      }),
    );
  });

  it('links to the ticket, shows the ITSM state and why delivery stopped, behind the icon', async () => {
    const user = userEvent.setup();
    renderWithProviders(<TicketsPanel />);
    await screen.findByText('INC0012345');
    expect(within(row()).getByRole('link', { name: /INC0012345/ })).toHaveAttribute(
      'href',
      'https://sn.example/INC0012345',
    );
    expect(within(row()).getByText('Acknowledged')).toBeInTheDocument();
    expect(within(row()).getByText('Stopped retrying')).toBeInTheDocument();
    expect(within(row()).queryByText(/does not exist/)).toBeNull();
    await user.click(within(row()).getByRole('button', { name: /error/i }));
    expect(await screen.findByText('Assignment group "Ops" does not exist')).toBeInTheDocument();
  });

  it('retries a stopped delivery once, and only with alerts.manage', async () => {
    const user = userEvent.setup();
    const { unmount } = renderWithProviders(<TicketsPanel />);
    await screen.findByText('INC0012345');
    await user.click(within(row()).getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(retries).toHaveBeenCalledWith('l1'));
    unmount();

    grantPermissions(['alerts.view']);
    renderWithProviders(<TicketsPanel />);
    await screen.findByText('INC0012345');
    const retry = within(row()).getByRole('button', { name: 'Try again' });
    expect(retry).toBeDisabled();
    await user.hover(retry.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the alerts.manage permission')).not.toHaveLength(0);
  });
});

const channel: ItsmChannel = {
  id: 'ch1',
  name: 'ServiceNow prod',
  channelType: 'servicenow',
  webhookSupported: true,
  pollSupported: true,
  tokenSet: true,
  secretSet: true,
};

describe('Channel webhook', () => {
  const rotations = vi.fn();
  beforeEach(() => {
    rotations.mockReset();
    grantPermissions(['communications.view', 'communications.manage']);
  });

  it('rotates only after confirmation, once, and shows the new URL and secret a single time', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/itsm/channels/:id/webhook-token', async () => {
        rotations();
        await gate;
        return HttpResponse.json({
          success: true,
          data: {
            url: 'https://mch.example/api/incident-webhooks/tok123',
            secret: 'sec456',
          },
        });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<ChannelWebhookPanel channel={channel} />);
    await user.click(screen.getByRole('button', { name: 'Rotate inbound URL' }));
    expect(rotations).not.toHaveBeenCalled();
    const confirm = within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Rotate',
    });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    expect(await screen.findByDisplayValue('https://mch.example/api/incident-webhooks/tok123')).toBeInTheDocument();
    expect(screen.getByDisplayValue('sec456')).toBeInTheDocument();
    expect(rotations).toHaveBeenCalledTimes(1);
  });

  it('keeps the dialog and the server message when rotation fails', async () => {
    server.use(
      http.post('*/api/itsm/channels/:id/webhook-token', () =>
        HttpResponse.json({ success: false, error: 'storing token: database is locked' }, { status: 500 }),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<ChannelWebhookPanel channel={channel} />);
    await user.click(screen.getByRole('button', { name: 'Rotate inbound URL' }));
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Rotate',
      }),
    );
    expect(await screen.findByText('storing token: database is locked')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('says a one-way channel reports no ticket status and offers no inbound URL', () => {
    renderWithProviders(
      <ChannelWebhookPanel
        channel={{ ...channel, channelType: 'alertmanager_webhook', webhookSupported: false, pollSupported: false }}
      />,
    );
    expect(screen.getByText(/only sends to the ITSM/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /inbound URL/ })).toBeNull();
  });

  it('is disabled, naming communications.manage, without it', async () => {
    grantPermissions(['communications.view']);
    const user = userEvent.setup();
    renderWithProviders(<ChannelWebhookPanel channel={channel} />);
    const rotate = screen.getByRole('button', { name: 'Rotate inbound URL' });
    expect(rotate).toBeDisabled();
    await user.hover(rotate.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the communications.manage permission')).not.toHaveLength(0);
  });
});
