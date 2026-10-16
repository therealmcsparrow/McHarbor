// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import type { UseMutationResult } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import type { ContainerInspect } from '@core/types/docker';
import { renderWithProviders } from '../../../../test/render';
import type { NetworkConnectPayload } from './NetworkTab';
import { NetworkTabConfigSection, type ConfigPart } from './NetworkTabConfigSection';
import { NetworkTabPortsSection } from './NetworkTabPortsSection';

function mutation<V>(): UseMutationResult<unknown, Error, V, unknown> {
  // unavoidable cast: the sections read only mutate and isPending from a mutation
  return { mutate: vi.fn(), isPending: false } as unknown as UseMutationResult<unknown, Error, V, unknown>;
}

function headings(): string[] {
  return screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent ?? '');
}

function renderConfig(parts?: ConfigPart[]) {
  return renderWithProviders(
    <NetworkTabConfigSection
      availableNetworks={[]}
      connectMutation={mutation<NetworkConnectPayload>()}
      container={{ Id: 'c1', Config: {}, HostConfig: { NetworkMode: 'bridge' } } as unknown as ContainerInspect}
      disconnectMutation={mutation<string>()}
      dns={[]}
      dnsOptions={[]}
      dnsSearch={[]}
      editing={false}
      allowLiveNetworkActionsInEdit={false}
      parts={parts}
      extraHostKeys={[]}
      extraHosts={[]}
      joinableNetworks={[]}
      networks={[]}
      onAddExtraHost={() => {}}
      onExtraHostChange={() => {}}
      onRemoveExtraHost={() => {}}
      selectedNetwork=""
      setSelectedNetwork={() => {}}
    />,
  );
}

function renderPorts(part?: 'all' | 'mappings' | 'exposed') {
  return renderWithProviders(
    <NetworkTabPortsSection
      containerId="c1"
      allowConnectionTest={false}
      editing={false}
      exposedPorts={[]}
      portMappingKeys={[]}
      portMappings={[]}
      publishedPortMappings={[]}
      onAddPortMapping={() => {}}
      onPortMappingChange={() => {}}
      onRemovePortMapping={() => {}}
      part={part}
    />,
  );
}

describe('configuration editor network layout', () => {
  it('renders the left-column cards in the order asked for, without Networks', () => {
    renderConfig(['extraHosts', 'dns']);
    const order = headings();
    expect(order[0]).toBe('Extra Hosts');
    expect(order).not.toContain('Networks');
  });

  it('renders only Networks for the right-hand column, headed by the network mode', () => {
    renderConfig(['networks']);
    expect(headings()).toEqual(['Networks']);
    expect(screen.getByText('bridge')).toBeInTheDocument();
  });

  it('keeps every card in the default order for the detail page', () => {
    renderConfig();
    expect(headings()).toEqual(expect.arrayContaining(['Extra Hosts', 'Networks']));
    expect(headings()).not.toContain('Network Config');
  });

  it('splits port mappings and exposed ports so they can sit in different places', () => {
    const { unmount } = renderPorts('mappings');
    expect(headings()).toEqual(['Port Mappings']);
    unmount();
    renderPorts('exposed');
    expect(headings()).toEqual(['Exposed Ports']);
  });
});
