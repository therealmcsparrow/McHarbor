// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import SourcesPage from '../pages/SourcesPage';
import { EnforcementSettings } from './EnforcementSettings';
import type { DesiredStateSource } from '../hooks/useSources';
import type { ReconcilerSettings } from '../hooks/useReconcilerSettings';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const intent: DesiredStateSource = {
  id: 's1',
  name: 'intent',
  repoId: 'r1',
  branch: 'main',
  path: 'desired',
  envId: 'e1',
  enabled: true,
  lastCommit: '0123456789abcdef',
  lastSyncedAt: '2026-09-26T10:00:00Z',
  rowCount: 4,
  createdAt: '2026-09-20T00:00:00Z',
  lastError: 'desired/bad.yaml document 1: mode enforce cannot be set from Git; grant it in McHarbor',
};

function mountSources() {
  if (!document.getElementById('pagehead-slot')) {
    const slot = document.createElement('div');
    slot.id = 'pagehead-slot';
    document.body.appendChild(slot);
  }
  return renderWithProviders(
    <MemoryRouter>
      <SourcesPage />
    </MemoryRouter>,
  );
}

function intentRow() {
  return screen.getByText('intent').closest('tr') as HTMLElement;
}

describe('Git sources', () => {
  const deletes = vi.fn();
  beforeEach(() => {
    deletes.mockReset();
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    server.use(
      http.get('*/api/reconciler/sources', () => HttpResponse.json({ success: true, data: [intent] })),
      http.delete('*/api/reconciler/sources/:id', ({ params }) => {
        deletes(params.id);
        return new HttpResponse(null, { status: 204 });
      }),
    );
  });

  it('shows why a sync failed behind the row icon, in full', async () => {
    const user = userEvent.setup();
    mountSources();
    await screen.findByText('intent');
    expect(within(intentRow()).getByText('Failing')).toBeInTheDocument();
    expect(within(intentRow()).queryByText(/cannot be set from Git/)).toBeNull();
    await user.click(within(intentRow()).getByRole('button', { name: /error/i }));
    expect(await screen.findByText(intent.lastError as string)).toBeInTheDocument();
  });

  it('deletes only after confirmation, once on a double click, and keeps the dialog with the server message on failure', async () => {
    const user = userEvent.setup();
    mountSources();
    await screen.findByText('intent');
    await user.click(within(intentRow()).getByRole('button', { name: 'Delete' }));
    expect(deletes).not.toHaveBeenCalled();
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.delete('*/api/reconciler/sources/:id', async ({ params }) => {
        deletes(params.id);
        await gate;
        return HttpResponse.json(
          { success: false, error: 'releasing source rows: database is locked' },
          { status: 500 },
        );
      }),
    );
    const confirm = screen.getByRole('button', { name: 'Delete source' });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('releasing source rows: database is locked')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete source' })).toBeInTheDocument();
  });

  it('renders sync and delete disabled, naming reconciler.manage, for a viewer', async () => {
    grantPermissions(['reconciler.view']);
    const user = userEvent.setup();
    mountSources();
    await screen.findByText('intent');
    for (const name of ['Sync now', 'Delete']) {
      expect(within(intentRow()).getByRole('button', { name })).toBeDisabled();
    }
    await user.hover(within(intentRow()).getByRole('button', { name: 'Delete' }).parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the reconciler.manage permission')).not.toHaveLength(0);
  });
});

const tripped: ReconcilerSettings = {
  confirmPasses: 2,
  enforceEnabled: true,
  maxActionsPerPass: 3,
  cooldownSeconds: 600,
  failureThreshold: 3,
  breakerOpen: true,
  breakerReason: 'web: daemon said no',
  breakerOpenedAt: '2026-09-26T04:00:00Z',
  consecutiveFailures: 3,
};

describe('Enforcement breaker', () => {
  const resets = vi.fn();
  beforeEach(() => {
    resets.mockReset();
    grantPermissions(['reconciler.view', 'reconciler.manage', 'reconciler.enforce']);
    server.use(
      http.post('*/api/reconciler/enforcement/reset', async () => {
        resets();
        return HttpResponse.json({ success: true, data: { ...tripped, breakerOpen: false, consecutiveFailures: 0 } });
      }),
    );
  });

  it('shows why enforcement stopped and resumes only after a confirmation, once', async () => {
    const user = userEvent.setup();
    renderWithProviders(<EnforcementSettings settings={tripped} />);
    expect(screen.getByText('web: daemon said no')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Reset and resume' }));
    expect(resets).not.toHaveBeenCalled();
    const confirm = screen.getByRole('button', { name: 'Resume enforcement' });
    await user.dblClick(confirm);
    await waitFor(() => expect(resets).toHaveBeenCalledTimes(1));
  });

  it('cannot be reset, and says which permission is missing, without reconciler.enforce', async () => {
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    const user = userEvent.setup();
    renderWithProviders(<EnforcementSettings settings={tripped} />);
    const reset = screen.getByRole('button', { name: 'Reset and resume' });
    expect(reset).toBeDisabled();
    await user.hover(reset.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the reconciler.enforce permission')).not.toHaveLength(0);
    expect(screen.getByRole('switch')).toBeDisabled();
  });
});
