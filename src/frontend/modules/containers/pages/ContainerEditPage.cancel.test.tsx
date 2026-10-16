// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ContainerInspect } from '@core/types/docker';
import { renderWithProviders } from '../../../test/render';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import type { useContainerConfig } from '../hooks/useContainerConfig';
import ContainerEditPage from './ContainerEditPage';

const container = {
  Id: 'c1',
  Name: '/web',
  Config: { Image: 'nginx', Labels: {} },
  State: { Status: 'running' },
  HostConfig: {},
  Mounts: [],
} as unknown as ContainerInspect;

const config = {
  editing: true,
  data: null,
  dirty: true,
  isSaving: false,
  changes: { hasResourceChanges: false, hasConfigChanges: false, changedResourceFields: [], changedConfigFields: [] },
  start: vi.fn(),
  cancel: vi.fn(),
  save: vi.fn(),
  setField: vi.fn(),
} as unknown as ReturnType<typeof useContainerConfig>;

vi.mock('../hooks/useContainers', () => ({
  useContainer: () => ({ data: container, isLoading: false, isError: false, error: null, refetch: vi.fn() }),
}));
vi.mock('../hooks/useContainerConfig', () => ({ useContainerConfig: () => config }));
vi.mock('../components/ContainerConfigEditShell', () => ({ ContainerConfigEditShell: () => null }));

describe('container configuration page Cancel', () => {
  let band: HTMLDivElement;

  beforeEach(() => {
    vi.clearAllMocks();
    grantPermissions(['containers.manage']);
    band = document.createElement('div');
    band.id = 'pagehead-slot';
    document.body.appendChild(band);
  });

  afterEach(() => {
    band.remove();
    resetPermissions();
  });

  it('discards the edit and returns to the container from the header Cancel', async () => {
    const user = userEvent.setup();
    const { findByText } = renderWithProviders(
      <MemoryRouter initialEntries={['/containers/c1/edit']}>
        <Routes>
          <Route path="/containers/:id/edit" element={<ContainerEditPage />} />
          <Route path="/containers/:id" element={<p>details page</p>} />
        </Routes>
      </MemoryRouter>,
    );
    await user.click(await within(band).findByRole('button', { name: 'Cancel' }));
    expect(config.cancel).toHaveBeenCalledTimes(1);
    expect(await findByText('details page')).toBeInTheDocument();
  });

  it('saves from the header Save', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <MemoryRouter initialEntries={['/containers/c1/edit']}>
        <Routes>
          <Route path="/containers/:id/edit" element={<ContainerEditPage />} />
        </Routes>
      </MemoryRouter>,
    );
    await user.click(await within(band).findByRole('button', { name: 'Save Changes' }));
    expect(config.save).toHaveBeenCalledTimes(1);
  });
});
