// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { EnvironmentDetailHeader } from './EnvironmentDetailHeader';
import { EnvironmentCardGrid } from './EnvironmentCardGrid';
import type { EnvironmentInfo } from '../hooks/useEnvironments';
import type { EnvironmentListItem } from '../hooks/useEnvironmentActions';

const env = {
  id: 'env-1',
  name: 'HomeLAB',
  connectionType: 'agent',
  agentHostname: 'homelab',
  isDefault: false,
} as unknown as EnvironmentInfo;

function headerProps() {
  return {
    env,
    status: 'connected',
    onTestConnection: vi.fn(),
    testPending: false,
    onRegenerateToken: vi.fn(),
    regeneratePending: false,
    onEdit: vi.fn(),
  };
}

describe('environment detail head', () => {
  let band: HTMLDivElement;

  beforeEach(() => {
    band = document.createElement('div');
    band.id = 'pagehead-slot';
    document.body.appendChild(band);
  });

  afterEach(() => {
    band.remove();
    resetPermissions();
  });

  it('puts the identity and the action buttons in the page-head band', async () => {
    grantPermissions(['environments.manage']);
    renderWithProviders(<EnvironmentDetailHeader {...headerProps()} />);
    await waitFor(() => expect(within(band).getByRole('heading', { name: 'HomeLAB' })).toBeInTheDocument());
    expect(within(band).getByRole('button', { name: 'Edit environment' })).toBeInTheDocument();
    expect(within(band).getByRole('button', { name: 'Test Connection' })).toBeInTheDocument();
    expect(band.children).toHaveLength(1);
    expect(within(band).getAllByRole('heading', { name: 'HomeLAB' })).toHaveLength(1);
  });
});

describe('environment card grid', () => {
  const items = [
    { id: 'a', name: 'Alpha', connectionType: 'socket' },
    { id: 'b', name: 'Beta', connectionType: 'agent' },
  ] as unknown as EnvironmentListItem[];

  it('renders exactly the list it is given and has no search box of its own', () => {
    renderWithProviders(
      <MemoryRouter>
        <EnvironmentCardGrid
          environments={items.slice(0, 1)}
          hasEnvironments
          isLoading={false}
          onTest={vi.fn()}
          onInstall={vi.fn()}
          onRemove={vi.fn()}
        />
      </MemoryRouter>,
    );
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.queryByText('Beta')).not.toBeInTheDocument();
  });

  it('says nothing matches when the search filtered everything out', () => {
    renderWithProviders(
      <EnvironmentCardGrid
        environments={[]}
        hasEnvironments
        isLoading={false}
        onTest={vi.fn()}
        onInstall={vi.fn()}
        onRemove={vi.fn()}
      />,
    );
    expect(screen.getByText('No environments match your search')).toBeInTheDocument();
    expect(screen.queryByText('No environments configured')).not.toBeInTheDocument();
  });
});
