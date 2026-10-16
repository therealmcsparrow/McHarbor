// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { useEnvironmentStore } from '@resources/stores/environment';
import ReconcilerPage from '../pages/ReconcilerPage';
import { isCreatable } from './CreateDesiredStateDialog';
import { driftClasses } from './useReconcilerColumns';
import type { DesiredState } from '../hooks/useReconciler';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const drifted: DesiredState = {
  id: 'ds1',
  mode: 'observe',
  name: 'shop watch',
  description: '',
  kind: 'stack',
  stackName: 'shop',
  containerName: '',
  imageRef: '',
  desiredStatus: 'running',
  restartPolicy: '',
  lastReconcile: '2026-09-26T10:00:00Z',
  driftDetected: true,
  createdAt: '2026-09-20T00:00:00Z',
  lastReport: {
    desiredStateId: 'ds1',
    hasDrift: true,
    checkedAt: '2026-09-26T10:00:00Z',
    diffs: [
      {
        class: 'state',
        field: 'service.db',
        expected: 'running',
        actual: 'exited',
        firstSeenAt: '2026-09-25T10:00:00Z',
      },
      {
        class: 'existence',
        field: 'service.sidecar',
        expected: 'not declared',
        actual: 'shop-sidecar-1',
      },
    ],
  },
};

function mountPage(states: DesiredState[]) {
  server.use(
    http.get('*/api/reconciler', () =>
      HttpResponse.json({
        success: true,
        data: {
          items: states,
          total: states.length,
          page: 1,
          per_page: 100,
          total_pages: 1,
        },
      }),
    ),
    http.get('*/api/table-views/reconciler', () =>
      HttpResponse.json({ success: true, data: { layout: null, filters: [] } }),
    ),
    http.get('*/api/stacks', () =>
      HttpResponse.json({
        success: true,
        data: [{ name: 'shop' }, { name: 'blog' }],
      }),
    ),
  );
  if (!document.getElementById('pagehead-slot')) {
    const slot = document.createElement('div');
    slot.id = 'pagehead-slot';
    document.body.appendChild(slot);
  }
  return renderWithProviders(
    <MemoryRouter>
      <ReconcilerPage />
    </MemoryRouter>,
  );
}

describe('Reconciler drift, Phase 1', () => {
  it('shows each drift class and opens the list of what drifted', async () => {
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    const user = userEvent.setup();
    mountPage([drifted]);
    await user.click(await screen.findByRole('button', { name: 'Show what drifted' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('service.db')).toBeInTheDocument();
    expect(within(dialog).getByText('exited')).toBeInTheDocument();
    expect(within(dialog).getByText('shop-sidecar-1')).toBeInTheDocument();
    expect(within(dialog).getByText('State')).toBeInTheDocument();
    expect(within(dialog).getByText('Existence')).toBeInTheDocument();
  });

  it('never offers to reconcile a stack or environment watch', async () => {
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    mountPage([drifted]);
    const button = await screen.findByRole('button', {
      name: 'Stack and environment watches cannot be reconciled directly; propose a fix instead.',
    });
    expect(button).toBeDisabled();
  });

  it('creates a stack watch with the chosen stack in the current environment', async () => {
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    useEnvironmentStore.setState({ currentId: 'env-7' });
    const seen = vi.fn();
    server.use(
      http.post('*/api/reconciler', async ({ request }) => {
        seen(await request.json());
        return HttpResponse.json({ success: true, data: { id: 'new' } });
      }),
    );
    const user = userEvent.setup();
    mountPage([]);
    await user.click(await screen.findByRole('button', { name: /Add/ }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getAllByRole('textbox')[0] as HTMLElement, 'shop watch');
    await user.click(within(dialog).getByRole('button', { name: 'What to watch' }));
    await user.click(await screen.findByRole('button', { name: 'A stack' }));
    const create = within(dialog).getByRole('button', { name: 'Create' });
    expect(create).toBeDisabled();
    await user.click(within(dialog).getByRole('button', { name: 'Stack' }));
    await user.click(await screen.findByRole('button', { name: 'shop' }));
    await user.click(create);
    await waitFor(() => expect(seen).toHaveBeenCalledTimes(1));
    expect(seen).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'shop watch',
        kind: 'stack',
        stackName: 'shop',
        envId: 'env-7',
      }),
    );
  });

  it('decides creatability per kind and derives distinct classes', () => {
    const base = {
      name: 'w',
      containerName: '',
      imageRef: '',
      stackName: '',
      desiredStatus: 'running',
      mode: 'observe' as const,
    };
    expect(isCreatable({ ...base, kind: 'container' })).toBe(false);
    expect(
      isCreatable({
        ...base,
        kind: 'container',
        containerName: 'web',
        imageRef: 'nginx',
      }),
    ).toBe(true);
    expect(isCreatable({ ...base, kind: 'stack' })).toBe(false);
    expect(isCreatable({ ...base, kind: 'environment' })).toBe(true);
    expect(driftClasses(drifted)).toEqual(['state', 'existence']);
  });
});
