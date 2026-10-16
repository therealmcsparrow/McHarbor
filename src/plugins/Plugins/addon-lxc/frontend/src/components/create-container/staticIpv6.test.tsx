// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, render, renderHook, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NetworkStep } from './NetworkStep';
import { useCreateContainerForm } from './useCreateContainerForm';
import type { ComputeNetwork } from '../../hooks/useComputeNetworks';

const state = vi.hoisted(() => ({
  networks: [] as ComputeNetwork[],
  mutate: vi.fn(),
}));

vi.mock('../../hooks/useHostEnvironment', () => ({ useHostEnvironment: () => 'env-a' }));
vi.mock('../../hooks/useContainers', () => ({ useCreateContainer: () => ({ mutate: state.mutate }) }));
vi.mock('../../hooks/useComputeNetworks', () => ({
  useComputeNetworks: () => ({ data: state.networks }),
}));

const macvlan: ComputeNetwork = {
  id: 'net-m',
  environmentId: 'env-a',
  name: 'direct',
  mode: 'macvlan',
  bridge: '',
  uplinkLink: 'eth0',
  linkMode: 'bridge',
  isDefault: false,
};
const bridged: ComputeNetwork = { ...macvlan, id: 'net-b', name: 'br', mode: 'bridge', bridge: 'br0' };

function fill(networks: ComputeNetwork[], networkId: string) {
  state.networks = networks;
  const hook = renderHook(() => useCreateContainerForm([{ id: 'env-a', name: 'a' }], vi.fn()));
  act(() => {
    hook.result.current.setNetworkId(networkId);
    hook.result.current.setStaticIpv6('2001:db8::50/64');
    hook.result.current.setStaticGw6('2001:db8::1');
  });
  return hook;
}

function submitted() {
  return state.mutate.mock.calls[0]?.[0].nics[0];
}

describe('static IPv6 on the create wizard', () => {
  beforeEach(() => state.mutate.mockClear());

  it('sends the pair for a macvlan network that does not say IPv6 is off', () => {
    const hook = fill([macvlan], 'net-m');
    act(() => hook.result.current.submit());
    expect(submitted()).toMatchObject({ staticIpv6: '2001:db8::50/64', staticGw6: '2001:db8::1' });
  });

  it('drops the pair when the network reports IPv6 disabled', () => {
    const hook = fill([{ ...macvlan, ipv6Enabled: false }], 'net-m');
    act(() => hook.result.current.submit());
    expect(submitted().staticIpv6).toBeUndefined();
  });

  it('drops the pair for a bridged network', () => {
    const hook = fill([bridged], 'net-b');
    act(() => hook.result.current.submit());
    expect(submitted().staticIpv6).toBeUndefined();
    expect(submitted().staticGw6).toBeUndefined();
  });

  it('shows the IPv6 fields for macvlan only', () => {
    state.networks = [macvlan, bridged];
    const hook = renderHook(() => useCreateContainerForm([{ id: 'env-a', name: 'a' }], vi.fn()));
    act(() => hook.result.current.setNetworkId('net-m'));
    const shown = render(<NetworkStep form={hook.result.current} />);
    expect(screen.getByLabelText('Static IPv6 (CIDR)')).toBeInTheDocument();
    shown.unmount();
    act(() => hook.result.current.setNetworkId('net-b'));
    render(<NetworkStep form={hook.result.current} />);
    expect(screen.queryByLabelText('Static IPv6 (CIDR)')).toBeNull();
  });
});
