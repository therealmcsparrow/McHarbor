// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { EMPTY_HOST_LINK, hostLinkRequestOf, type HostLinkFormValue } from '../utils/hostLinkForm';
import { HostLinkFields } from './HostLinkFields';

function answer(data: object) {
  server.use(
    http.get('*/api/networks/host-interfaces', () =>
      HttpResponse.json({ success: true, data: { interfaces: [], agentAvailable: true, ...data } }),
    ),
  );
}

function Harness({
  enableIPv6,
  mode = 'bridge',
  initial = EMPTY_HOST_LINK,
}: {
  enableIPv6: boolean;
  mode?: string;
  initial?: HostLinkFormValue;
}) {
  const [value, setValue] = useState(initial);
  return <HostLinkFields value={value} onChange={setValue} enableIPv6={enableIPv6} mode={mode} />;
}

describe('HostLinkFields', () => {
  it('is disabled with the reason when the environment cannot manage host interfaces', async () => {
    answer({ hostLinkAvailable: false, hostLinkUnavailable: 'agent_outdated' });
    renderWithProviders(<Harness enableIPv6={false} />);
    expect(await screen.findByText(/too old to manage host interfaces/)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Create a host interface' })).toBeDisabled();
  });

  it('explains a containerised McHarbor instead of offering the switch', async () => {
    answer({ hostLinkAvailable: false, hostLinkUnavailable: 'containerized' });
    renderWithProviders(<Harness enableIPv6={false} />);
    expect(await screen.findByText(/runs inside a container here/)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Create a host interface' })).toBeDisabled();
  });

  it('is blocked in passthru mode even where the host could do it', async () => {
    answer({ hostLinkAvailable: true });
    renderWithProviders(<Harness enableIPv6={false} mode="passthru" />);
    expect(await screen.findByText(/Not available in passthru mode/)).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Create a host interface' })).toBeDisabled();
  });

  it('offers SLAAC only when the network has IPv6', async () => {
    answer({ hostLinkAvailable: true });
    const user = userEvent.setup();
    const { rerender } = renderWithProviders(<Harness enableIPv6={false} />);
    const toggle = await screen.findByRole('switch', { name: 'Create a host interface' });
    await user.click(toggle);
    expect(screen.getByPlaceholderText('mcvlan0')).toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Automatic (SLAAC)' })).not.toBeInTheDocument();
    expect(screen.queryByText('Host IPv6 address')).not.toBeInTheDocument();

    rerender(<Harness enableIPv6 initial={{ ...EMPTY_HOST_LINK, enabled: true }} />);
    const auto = await screen.findByRole('switch', { name: 'Automatic (SLAAC)' });
    const v6 = screen.getByPlaceholderText(/2001:db8:1::250/);
    expect(v6).toBeEnabled();
    await user.click(auto);
    expect(v6).toBeDisabled();
  });
});

describe('hostLinkRequestOf', () => {
  const on: HostLinkFormValue = {
    enabled: true,
    name: ' mcvlan0 ',
    ipv4: ' 192.168.1.250/24 ',
    ipv6: '2001:db8:1::250/64',
    ipv6Auto: false,
  };

  it('asks for nothing while the switch is off', () => {
    expect(hostLinkRequestOf({ ...on, enabled: false }, true)).toBeUndefined();
  });

  it('trims, and sends an IPv6 address only for a network with IPv6', () => {
    expect(hostLinkRequestOf(on, true)).toEqual({
      name: 'mcvlan0',
      ipv4: '192.168.1.250/24',
      ipv6: '2001:db8:1::250/64',
    });
    expect(hostLinkRequestOf(on, false)).toEqual({ name: 'mcvlan0', ipv4: '192.168.1.250/24' });
  });

  it('sends SLAAC instead of the typed address, never both', () => {
    expect(hostLinkRequestOf({ ...on, ipv6Auto: true }, true)).toEqual({
      name: 'mcvlan0',
      ipv4: '192.168.1.250/24',
      ipv6Auto: true,
    });
    expect(hostLinkRequestOf({ ...on, ipv6Auto: true }, false)).toEqual({ name: 'mcvlan0', ipv4: '192.168.1.250/24' });
  });
});
