// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RemoveContainerDialog } from './RemoveContainerDialog';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const target = { id: 'c1', name: 'web', image: 'nginx:1.27', imageId: 'sha256:1', stackName: null };

function mount(overrides: Partial<{ onOpenChange: (open: boolean) => void; onSuccess: () => void }> = {}) {
  const onOpenChange = overrides.onOpenChange ?? vi.fn();
  const onSuccess = overrides.onSuccess ?? vi.fn();
  const view = renderWithProviders(
    <RemoveContainerDialog container={target} open onOpenChange={onOpenChange} onSuccess={onSuccess} />,
  );
  return { ...view, onOpenChange, onSuccess };
}

function removeButton() {
  return screen.getByRole('button', { name: 'Remove' });
}

describe('RemoveContainerDialog', () => {
  const removals = vi.fn();

  beforeEach(() => {
    removals.mockReset();
    grantPermissions(['containers.delete']);
    server.use(
      http.get('*/api/containers/c1', () =>
        HttpResponse.json({ success: true, data: { Id: 'c1', NetworkSettings: { Networks: {} } } }),
      ),
      http.get('*/api/networks', () => HttpResponse.json({ success: true, data: [] })),
      http.post('*/api/containers/c1/remove', async ({ request }) => {
        removals(await request.json());
        return HttpResponse.json({ success: true });
      }),
    );
  });

  it('does not remove anything when cancelled', async () => {
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(removals).not.toHaveBeenCalled();
  });

  it('removes once, with the chosen options, only after confirmation', async () => {
    const user = userEvent.setup();
    const { onSuccess } = mount();
    expect(removals).not.toHaveBeenCalled();
    await user.click(screen.getByRole('switch', { name: /Remove volumes/ }));
    await user.click(removeButton());
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1));
    expect(removals).toHaveBeenCalledTimes(1);
    expect(removals).toHaveBeenCalledWith({
      force: true,
      removeVolumes: true,
      removeImage: false,
      removeStack: false,
      removeNetwork: false,
    });
  });

  it('does not fire twice on a double click', async () => {
    let release: () => void = () => undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/containers/c1/remove', async ({ request }) => {
        removals(await request.json());
        await held;
        return HttpResponse.json({ success: true });
      }),
    );
    const user = userEvent.setup();
    const { onSuccess } = mount();
    const button = removeButton();
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(removals).toHaveBeenCalledTimes(1);
  });

  it('renders the confirm control disabled, naming the permission, when containers.delete is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    expect(removeButton()).toBeDisabled();
    await user.click(removeButton());
    expect(removals).not.toHaveBeenCalled();
    await user.hover(removeButton().parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the containers.delete permission')).not.toHaveLength(0);
  });

  it('keeps the dialog open and reports the server message when removal fails', async () => {
    server.use(
      http.post('*/api/containers/c1/remove', () =>
        HttpResponse.json(
          { success: false, error: 'container c1 is protected by a hold', code: 'err.container.protected' },
          { status: 409 },
        ),
      ),
    );
    const user = userEvent.setup();
    const { onOpenChange, onSuccess, client } = mount();
    await user.click(removeButton());
    await waitFor(() => expect(client.getMutationCache().getAll()[0]?.state.status).toBe('error'));
    expect(client.getMutationCache().getAll()[0]?.state.error?.message).toBe('container c1 is protected by a hold');
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(removeButton()).toBeEnabled();
  });
});
