// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { DestroyPreviewDialog } from './DestroyPreviewDialog';
import type { GitOpsPRPreview } from '../types';

const preview: GitOpsPRPreview = {
  id: 'prev-1',
  pipelineId: 'pipe-1',
  repoId: 'repo-1',
  prNumber: '42',
  prTitle: 'Add login',
  sourceBranch: 'feature/login',
  targetBranch: 'main',
  commitSha: 'deadbeef',
  previewEnvironmentId: 'env-1',
  stackName: 'preview-web-pr-42',
  status: 'active',
  url: 'https://pr-42.preview.example.com',
  expiresAt: '2026-10-01T00:00:00Z',
  tornDownBy: '',
  lastError: '',
  commentError: '',
  openedAt: '2026-09-27T00:00:00Z',
  closedAt: '',
};

describe('DestroyPreviewDialog', () => {
  const calls = vi.fn();

  function answer(response: () => Response | Promise<Response>) {
    server.use(
      http.delete('*/api/gitops/previews/prev-1', () => {
        calls();
        return response();
      }),
    );
  }

  function mount() {
    const onOpenChange = vi.fn();
    renderWithProviders(<DestroyPreviewDialog open preview={preview} onOpenChange={onOpenChange} />);
    return { onOpenChange };
  }

  const confirmButton = async () =>
    within(await screen.findByRole('dialog')).getByRole('button', { name: 'Destroy preview' });

  beforeEach(() => {
    calls.mockReset();
    grantPermissions(['gitops.delete']);
    answer(() => HttpResponse.json({ success: true, data: { ...preview, status: 'demolished' } }));
  });

  it('sends nothing until the pull request is typed', async () => {
    const user = userEvent.setup();
    mount();
    const confirm = await confirmButton();
    expect(confirm).toBeDisabled();
    await user.click(confirm);
    expect(calls).not.toHaveBeenCalled();

    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox'), 'PR #41');
    expect(await confirmButton()).toBeDisabled();
    expect(calls).not.toHaveBeenCalled();
  });

  it('sends exactly one request on a double click with the response held open', async () => {
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    answer(async () => {
      await held;
      return HttpResponse.json({ success: true, data: { ...preview, status: 'demolished' } });
    });

    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox'), 'PR #42');
    const confirm = await confirmButton();
    await user.click(confirm);
    await user.click(confirm);
    expect(calls).toHaveBeenCalledTimes(1);
    release();
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(calls).toHaveBeenCalledTimes(1);
  });

  it('renders the confirm disabled, naming gitops.delete, when it is missing', async () => {
    grantPermissions([]);
    const user = userEvent.setup();
    mount();
    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox'), 'PR #42');
    const confirm = await confirmButton();
    expect(confirm).toBeDisabled();
    expect(confirm).toHaveAttribute('aria-disabled', 'true');
    await user.hover(confirm);
    expect(await screen.findAllByText(/gitops\.delete/)).not.toHaveLength(0);
    await user.click(confirm);
    expect(calls).not.toHaveBeenCalled();
  });

  it("shows the server's own message intact and keeps the dialog open", async () => {
    const message = 'the environments service is unavailable, teardown will be retried';
    answer(() => HttpResponse.json({ success: false, error: message }, { status: 500 }));
    const user = userEvent.setup();
    const { onOpenChange } = mount();
    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox'), 'PR #42');
    await user.click(await confirmButton());
    const alerts = await screen.findAllByRole('alert');
    expect(alerts.map((node) => node.textContent ?? '').join(' ')).toContain(message);
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
