// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCreateDesiredState } from '../hooks/useReconciler';
import { CreateDesiredStateDialog } from './CreateDesiredStateDialog';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const vmKind = {
  source: 'addon-kvm',
  kind: 'virtual_machine',
  label: 'Virtual machine',
  subjectLabel: 'Virtual machine',
  enforceable: true,
  ownScopes: false,
  fields: [
    {
      key: 'power',
      label: 'Power',
      type: 'select',
      options: [
        { value: 'running', label: 'Running' },
        { value: 'stopped', label: 'Stopped' },
      ],
    },
    { key: 'memory_mib', label: 'Memory', type: 'number', unit: 'MiB' },
  ],
};

function Harness() {
  const create = useCreateDesiredState((k) => k);
  const [open, setOpen] = useState(true);
  return <CreateDesiredStateDialog open={open} onOpenChange={setOpen} createState={create} />;
}

describe('Desired state form', () => {
  let posted: Record<string, unknown>[];

  beforeEach(() => {
    posted = [];
    grantPermissions(['reconciler.view', 'reconciler.manage']);
    useEnvironmentStore.setState({ currentId: 'env-1', environments: [{ id: 'env-1', name: 'lab' }] as never });
    server.use(
      http.get('*/api/reconciler/kinds', () => HttpResponse.json({ success: true, data: [vmKind] })),
      http.get('*/api/containers', () =>
        HttpResponse.json({
          success: true,
          data: [{ Id: 'c1', Names: ['/web'], Image: 'nginx:1.27', ImageID: 'sha256:x', State: 'running' }],
        }),
      ),
      http.get('*/api/images', () =>
        HttpResponse.json({ success: true, data: [{ Id: 'i1', RepoTags: ['nginx:1.27', 'redis:7'] }] }),
      ),
      http.get('*/api/stacks', () => HttpResponse.json({ success: true, data: [] })),
      http.get('*/api/reconciler/kinds/addon-kvm/virtual_machine/subjects', () =>
        HttpResponse.json({ success: true, data: [{ value: 'vm-1', label: 'db-vm', detail: 'stopped · 2 vCPU' }] }),
      ),
      http.get('*/api/reconciler/kinds/addon-kvm/virtual_machine/current', () =>
        HttpResponse.json({ success: true, data: { power: 'stopped', memory_mib: '2048' } }),
      ),
      http.post('*/api/reconciler', async ({ request }) => {
        posted.push((await request.json()) as Record<string, unknown>);
        return HttpResponse.json({ success: true, data: { id: 'ds1' } }, { status: 201 });
      }),
    );
  });

  it('picks a container from a list and takes its image and state', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness />);
    await user.type(screen.getByPlaceholderText(/./, { exact: false }), 'web watch');
    await user.click(screen.getByRole('button', { name: /Container name|Container/ }));
    await user.click(await screen.findByRole('button', { name: /^web/ }));
    await user.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: 'container',
      containerName: 'web',
      imageRef: 'nginx:1.27',
      desiredStatus: 'running',
    });
  });

  it('lets a name be typed when the container does not exist yet', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness />);
    await user.type(screen.getByPlaceholderText(/./, { exact: false }), 'future');
    await user.click(screen.getByRole('button', { name: /Container name|Container/ }));
    await user.click(await screen.findByRole('button', { name: 'Enter manually…' }));
    await user.type(screen.getByRole('textbox', { name: /Container name|Container/ }), 'not-yet');
    expect(screen.getByRole('button', { name: 'Pick from list' })).toBeInTheDocument();
  });

  it('builds a plugin desired state from selectors and the current values', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness />);
    await user.type(screen.getByPlaceholderText(/./, { exact: false }), 'db vm');
    await user.click(screen.getByRole('button', { name: 'What to watch' }));
    await user.click(await screen.findByRole('button', { name: /Virtual machine/ }));
    await user.click(await screen.findByRole('button', { name: 'Virtual machine' }));
    await user.click(await screen.findByRole('button', { name: /db-vm/ }));
    await user.click(await screen.findByRole('button', { name: 'Use current values' }));
    await waitFor(() => expect(screen.getByDisplayValue('2048')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Create' }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: 'virtual_machine',
      source: 'addon-kvm',
      subjectId: 'vm-1',
      envId: 'env-1',
      spec: { power: 'stopped', memory_mib: '2048' },
    });
  });
});
