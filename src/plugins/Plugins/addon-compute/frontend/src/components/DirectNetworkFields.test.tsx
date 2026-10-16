// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DirectNetworkFields } from './DirectNetworkFields';
import { defaultHostLinkName, hostLinkLabel } from '../utils/networkAttachment';
import type { HostLinkValues, Network } from '../types';

const START: HostLinkValues = {
  hostAccess: false,
  hostAccessIp: '',
  ipv6Enabled: false,
  hostAccessIpv6: '',
  hostLinkName: '',
};

function Harness({ initial = START, onChange }: { initial?: HostLinkValues; onChange?: (v: HostLinkValues) => void }) {
  const [hostLink, setHostLink] = useState(initial);
  return (
    <DirectNetworkFields
      mode="macvlan"
      parents={[]}
      parentsLoading={false}
      noHostNetwork={false}
      uplinkLink="eth0"
      onUplinkChange={vi.fn()}
      linkMode="bridge"
      onLinkModeChange={vi.fn()}
      vlanTag=""
      onVlanTagChange={vi.fn()}
      hostLink={hostLink}
      onHostLinkChange={(next) => {
        setHostLink(next);
        onChange?.(next);
      }}
      defaultHostLinkName="mcharbor-hab123"
    />
  );
}

describe('DirectNetworkFields host interface', () => {
  it('shows the name and address fields only once the host has an interface, and IPv6 only when it is on', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    expect(screen.queryByLabelText('Host interface name')).not.toBeInTheDocument();
    await user.click(screen.getByRole('switch', { name: 'Let this host reach its guests' }));

    expect(screen.getByLabelText('Host interface name')).toHaveAttribute('placeholder', 'mcharbor-hab123');
    expect(screen.queryByLabelText('IPv6 address for this host')).not.toBeInTheDocument();

    await user.click(screen.getByRole('switch', { name: 'IPv6' }));
    expect(screen.getByLabelText('IPv6 address for this host')).toBeInTheDocument();
  });

  it('clears what hangs off a switch when it is turned off, so no hidden value is sent', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <Harness
        onChange={onChange}
        initial={{
          hostAccess: true,
          hostAccessIp: '192.168.1.9/24',
          ipv6Enabled: true,
          hostAccessIpv6: 'fd00::9/64',
          hostLinkName: 'lanshim',
        }}
      />,
    );

    await user.click(screen.getByRole('switch', { name: 'IPv6' }));
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ ipv6Enabled: false, hostAccessIpv6: '', hostAccessIp: '192.168.1.9/24' }),
    );

    await user.click(screen.getByRole('switch', { name: 'Let this host reach its guests' }));
    expect(onChange).toHaveBeenLastCalledWith(START);
  });

  it('reports the typed name and IPv6 address', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} initial={{ ...START, hostAccess: true, ipv6Enabled: true }} />);

    await user.type(screen.getByLabelText('Host interface name'), 'lan0');
    await user.type(screen.getByLabelText('IPv6 address for this host'), 'fd00::9/64');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ hostLinkName: 'lan0', hostAccessIpv6: 'fd00::9/64' }),
    );
  });
});

describe('hostLinkLabel', () => {
  const network = {
    id: 'cabc1234567xyz9z',
    mode: 'macvlan',
    hostAccess: true,
    hostAccessIp: '192.168.1.9/24',
  } as Network;

  it('names the generated default when the operator named nothing', () => {
    expect(defaultHostLinkName(network.id)).toBe('mcharbor-hxyz9z');
    expect(hostLinkLabel(network)).toBe(`${defaultHostLinkName(network.id)} · 192.168.1.9/24`);
  });

  it('shows the chosen name with both families, SLAAC when no v6 address is set', () => {
    expect(hostLinkLabel({ ...network, hostLinkName: 'lanshim', ipv6Enabled: true })).toBe(
      'lanshim · 192.168.1.9/24 · SLAAC',
    );
    expect(
      hostLinkLabel({ ...network, hostLinkName: 'lanshim', ipv6Enabled: true, hostAccessIpv6: 'fd00::9/64' }),
    ).toBe('lanshim · 192.168.1.9/24 · fd00::9/64');
  });

  it('is empty without host access', () => {
    expect(hostLinkLabel({ ...network, hostAccess: false })).toBe('');
  });
});
