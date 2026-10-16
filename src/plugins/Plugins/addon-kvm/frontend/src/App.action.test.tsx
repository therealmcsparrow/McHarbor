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
vi.mock('./hooks/useVMs', () => ({ useVMs: () => ({ data: [] }) }));
vi.mock('./hooks/useHostEnvironments', () => ({ useHostEnvironmentNames: () => new Map() }));
vi.mock('./pages/VMDetailPage', () => ({ VMDetailPage: () => <div data-testid="detail" /> }));
vi.mock('./pages/VMListPage', () => ({
  VMListPage: (props: { importRequested?: boolean; onImportRequestDone?: () => void }) => (
    <div data-testid="list" data-import={String(Boolean(props.importRequested))}>
      <button onClick={props.onImportRequestDone}>finish-import</button>
    </div>
  ),
}));
vi.mock('./components/CreateVMDialog', () => ({
  CreateVMDialog: ({ onClose }: { onClose: () => void }) => (
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
    slug: 'vm',
    path: '/plugins/addon-kvm',
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

describe('addon-kvm ?action= handling', () => {
  beforeEach(() => {
    gate.allowed = true;
    navigate.mockClear();
  });

  it('opens the create dialog for ?action=create and drops the param when it closes', () => {
    mount('?action=create');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByText('close-create'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(navigate).toHaveBeenCalledWith('/plugins/addon-kvm', { replace: true });
  });

  it('asks the list to open its import flow for ?action=import and drops the param when done', () => {
    mount('?action=import');
    expect(screen.getByTestId('list')).toHaveAttribute('data-import', 'true');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('finish-import'));
    expect(navigate).toHaveBeenCalledWith('/plugins/addon-kvm', { replace: true });
  });

  it('ignores an unknown action', () => {
    mount('?action=bogus');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByTestId('list')).toHaveAttribute('data-import', 'false');
  });

  it('does not open anything without the permission to do it', () => {
    gate.allowed = false;
    mount('?action=create');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    mount('?action=import');
    expect(screen.getAllByTestId('list').every((el) => el.getAttribute('data-import') === 'false')).toBe(true);
  });

  it('ignores the action while a guest is open', () => {
    mount('?action=create', 'abc123');
    expect(screen.getByTestId('detail')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
