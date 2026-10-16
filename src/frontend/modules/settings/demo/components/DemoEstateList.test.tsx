// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DemoEstateList } from './DemoEstateList';
import { DEMO_DEFAULTS } from '../test-fixtures';
import { renderWithProviders } from '../../../../test/render';

function tile(label: string) {
  const term = screen.getByText(label, { selector: 'dt' });
  return within(term.parentElement as HTMLElement);
}

describe('DemoEstateList', () => {
  it('shows the configured sizes, Swarm and the history period when no demo is running', () => {
    const config = { ...DEMO_DEFAULTS, swarmClusters: 3, swarmNodes: 12, swarmServices: 75, historyDays: 45 };
    renderWithProviders(<DemoEstateList config={config} />);
    expect(screen.getByText('What the demo estate contains')).toBeInTheDocument();
    expect(tile('Swarm clusters').getByText('3')).toBeInTheDocument();
    expect(tile('Swarm nodes').getByText('12')).toBeInTheDocument();
    expect(tile('Swarm services').getByText('75')).toBeInTheDocument();
    expect(tile('History period').getByText('45 days')).toBeInTheDocument();
    expect(tile('Kubernetes deployments').getByText('264')).toBeInTheDocument();
    expect(tile('Containers').getByText('1,212')).toBeInTheDocument();
    expect(screen.queryByText('Pods')).not.toBeInTheDocument();
  });

  it('prefers the live counts while demo mode is active', () => {
    renderWithProviders(
      <DemoEstateList
        config={DEMO_DEFAULTS}
        counts={{
          environments: 16,
          containers: 1234,
          stacks: 151,
          virtualMachines: 121,
          systemContainers: 81,
          clusters: 3,
          pods: 412,
          swarmClusters: 2,
          swarmNodes: 9,
          swarmServices: 61,
          historyDays: 1,
        }}
      />,
    );
    expect(screen.getByText('What the demo estate contains right now')).toBeInTheDocument();
    expect(tile('Pods').getByText('412')).toBeInTheDocument();
    expect(tile('Swarm services').getByText('61')).toBeInTheDocument();
    expect(tile('Swarm nodes').getByText('9')).toBeInTheDocument();
    expect(tile('History period').getByText('1 day')).toBeInTheDocument();
    expect(screen.queryByText('Kubernetes deployments')).not.toBeInTheDocument();
  });
});
