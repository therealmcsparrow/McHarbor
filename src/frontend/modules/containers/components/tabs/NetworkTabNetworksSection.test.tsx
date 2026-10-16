// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import type { UseMutationResult } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { NetworkInfo } from '@core/types/docker';
import { renderWithProviders } from '../../../../test/render';
import { i18n } from '../../../../test/i18n';
import { grantPermissions, resetPermissions } from '../../../../test/permissions';
import type { NetworkConnectPayload } from './NetworkTab';
import type { ContainerNetworkEntry } from './NetworkTabNetworkCard';
import { NetworkTabNetworksSection } from './NetworkTabNetworksSection';

const t = i18n.getFixedT('en', 'containers');

function mutation<V>(): UseMutationResult<unknown, Error, V, unknown> {
  // unavoidable cast: the card reads only mutate and isPending from a mutation
  return { mutate: vi.fn(), isPending: false } as unknown as UseMutationResult<unknown, Error, V, unknown>;
}

const networks: ContainerNetworkEntry[] = [
  ['lan', { IPAddress: '192.168.1.50', MacAddress: '02:42:c0:a8:01:32', Aliases: ['web'] } as ContainerNetworkEntry[1]],
];
const joinable = [{ Name: 'backend', Id: 'n2' } as NetworkInfo];

function renderSection(props: { readOnly?: boolean; editing?: boolean; allowLiveActionsInEdit?: boolean }) {
  return renderWithProviders(
    <NetworkTabNetworksSection
      connectMutation={mutation<NetworkConnectPayload>()}
      disconnectMutation={mutation<string>()}
      editing={props.editing ?? false}
      readOnly={props.readOnly}
      allowLiveActionsInEdit={props.allowLiveActionsInEdit}
      joinableNetworks={joinable}
      networks={networks}
      selectedNetwork=""
      setSelectedNetwork={() => {}}
      t={t}
    />,
  );
}

describe('container networks card', () => {
  afterEach(() => resetPermissions());

  it('is read-only on the detail page: the networks show, no action does', () => {
    grantPermissions(['containers.manage']);
    renderSection({ readOnly: true });
    expect(screen.getByText('lan')).toBeInTheDocument();
    expect(screen.getByText(t('networkTab.readOnlyHint'))).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Disconnect/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Connect/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: t('networkTab.editNetworkSettings') })).not.toBeInTheDocument();
  });

  it('offers connect, endpoint edit and disconnect in the configuration editor', () => {
    grantPermissions(['containers.manage']);
    renderSection({ editing: true, allowLiveActionsInEdit: true });
    expect(screen.getByRole('button', { name: /Disconnect/ })).toBeEnabled();
    expect(screen.getByRole('button', { name: t('networkTab.editNetworkSettings') })).toBeEnabled();
    expect(screen.getByRole('button', { name: /^Connect/ })).toBeInTheDocument();
  });

  it('keeps the editor actions disabled without containers.manage', () => {
    grantPermissions([]);
    renderSection({ editing: true, allowLiveActionsInEdit: true });
    expect(screen.getByRole('button', { name: /Disconnect/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: t('networkTab.editNetworkSettings') })).toBeDisabled();
  });
});
