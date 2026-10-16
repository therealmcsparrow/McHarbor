// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import type { PluginNavContext } from './index';

const gate = vi.hoisted(() => ({ allowed: true }));

vi.mock('@resources/hooks/usePermissionGate', () => ({
  usePermissionGate: () => ({ allowed: gate.allowed }),
}));
vi.mock('./hooks/useContainers', () => ({ useContainers: () => ({ data: [] }) }));
vi.mock('./pages/ContainerDetailPage', () => ({
  ContainerDetailPage: () => <div data-testid="detail" />,
}));
vi.mock('./pages/ContainerListPage', () => ({ ContainerListPage: () => <div data-testid="list" /> }));
vi.mock('./components/CreateContainerDialog', () => ({
  CreateContainerDialog: ({ onClose }: { onClose: () => void }) => (
    <dialog open>
      <button onClick={onClose}>close-create</button>
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
    slug: 'ct',
    path: '/plugins/addon-lxc',
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

describe('addon-lxc ?action= handling', () => {
  beforeEach(() => {
    gate.allowed = true;
    navigate.mockClear();
  });

  it('opens the create dialog for ?action=create and drops the param when it closes', () => {
    mount('?action=create');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByText('close-create'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(navigate).toHaveBeenCalledWith('/plugins/addon-lxc', { replace: true });
  });

  it('ignores an unknown action', () => {
    mount('?action=bogus');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByTestId('list')).toBeInTheDocument();
  });

  it('does not open the dialog without the permission to create', () => {
    gate.allowed = false;
    mount('?action=create');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('ignores the action while a container is open', () => {
    mount('?action=create', 'abc123');
    expect(screen.getByTestId('detail')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
