// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { COMPUTE_PERM } from './permissions';
import type { PluginNavContext } from './index';

const gate = vi.hoisted(() => ({ denied: new Set<string>() }));

vi.mock('@resources/hooks/usePermissionGate', () => ({
  usePermissionGate: (key: string) => ({ allowed: !gate.denied.has(key) }),
}));
vi.mock('./hooks/useCompute', () => ({
  useEnvironments: () => ({ data: [], isLoading: false, isError: false, refetch: vi.fn() }),
}));
vi.mock('./hooks/useHostEnvironments', () => ({ useHostEnvironmentNames: () => new Map() }));
vi.mock('./pages/OverviewPage', () => ({ OverviewPage: () => <div data-testid="overview" /> }));
vi.mock('./pages/HostDetailPage', () => ({
  HOST_DETAIL_TABS: ['overview'],
  HostDetailPage: () => <div />,
}));
vi.mock('./pages/PoolDetailPage', () => ({ PoolDetailPage: () => <div /> }));
vi.mock('./pages/NetworksPage', () => ({ NetworksPage: () => <div /> }));
vi.mock('./pages/PoolsPage', () => ({
  PoolsPage: (props: { createRequested?: boolean; onCreateRequestDone?: () => void }) => (
    <div data-testid="pools" data-create={String(Boolean(props.createRequested))}>
      <button onClick={props.onCreateRequestDone}>finish-create</button>
    </div>
  ),
}));
vi.mock('./pages/ImagesPage', () => ({
  ImagesPage: (props: { uploadRequested?: boolean }) => (
    <div data-testid="images" data-upload={String(Boolean(props.uploadRequested))} />
  ),
}));
vi.mock('./components/SetupWizard', () => ({
  SetupWizard: ({ onClose }: { onClose: () => void }) => (
    <dialog open>
      <button onClick={onClose}>close-wizard</button>
    </dialog>
  ),
}));

let listener: ((ctx: PluginNavContext) => void) | null = null;
let current: PluginNavContext;

// Mirrors the host: navigating re-delivers the new location through subscribe().
const navigate = vi.fn((to: string) => {
  const [path = '', search = ''] = to.split('?');
  listener?.({ ...current, path, search: search ? `?${search}` : '' });
});

function mount(search: string, splat = '') {
  const initial: PluginNavContext = {
    slug: 'compute',
    path: '/plugins/addon-compute',
    splat,
    search,
    navigate,
  };
  current = initial;
  return render(
    <App
      initial={initial}
      subscribe={(l) => {
        listener = l;
        return () => undefined;
      }}
    />,
  );
}

describe('addon-compute ?action= handling', () => {
  beforeEach(() => {
    gate.denied.clear();
    navigate.mockClear();
  });

  it('opens the setup wizard for ?action=setup on the overview and drops the param when it closes', () => {
    mount('?action=setup');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByText('close-wizard'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(navigate).toHaveBeenCalledWith('/plugins/addon-compute', { replace: true });
  });

  it('asks the pools page to open create for ?action=create and drops the param when done', () => {
    mount('?action=create', 'pools');
    expect(screen.getByTestId('pools')).toHaveAttribute('data-create', 'true');
    fireEvent.click(screen.getByText('finish-create'));
    expect(navigate).toHaveBeenCalledWith('/plugins/addon-compute', { replace: true });
  });

  it('asks the images page to open upload for ?action=upload', () => {
    mount('?action=upload', 'images');
    expect(screen.getByTestId('images')).toHaveAttribute('data-upload', 'true');
  });

  it('ignores an unknown action', () => {
    mount('?action=bogus');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByTestId('overview')).toBeInTheDocument();
  });

  it('does not act on a request the user lacks the permission for', () => {
    gate.denied.add(COMPUTE_PERM.MANAGE);
    gate.denied.add(COMPUTE_PERM.IMAGES_UPLOAD);
    mount('?action=setup');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    mount('?action=create', 'pools');
    expect(screen.getByTestId('pools')).toHaveAttribute('data-create', 'false');
    mount('?action=upload', 'images');
    expect(screen.getByTestId('images')).toHaveAttribute('data-upload', 'false');
  });

  it('only honours setup on the overview, not on another page', () => {
    mount('?action=setup', 'pools');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
