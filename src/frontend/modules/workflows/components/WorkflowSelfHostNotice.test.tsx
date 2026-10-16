// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { Button } from '@resources/components/ui/Button';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { useUpdateWorkflow } from '../hooks/useWorkflows';
import { SelfHostAuthorizedBadge, WorkflowSaveRefusal } from './WorkflowSelfHostNotice';

const SERVER_MESSAGE =
  "This workflow has a node that acts on McHarbor's own containers, stacks or host. Saving it needs the host_self.manage permission, because the workflow later runs for the system with nobody to ask";

function SaveHarness() {
  const update = useUpdateWorkflow();
  return (
    <>
      <Button onClick={() => update.mutate({ id: 'wf1', canvasData: '{}' })}>save</Button>
      <WorkflowSaveRefusal error={update.error} />
    </>
  );
}

describe('workflow editor save refusal', () => {
  beforeEach(() => grantPermissions(['workflows.view', 'workflows.manage']));

  it('shows the server refusal verbatim when a save is refused for host_self.manage', async () => {
    server.use(
      http.put('*/api/workflows/wf1', () =>
        HttpResponse.json(
          {
            success: false,
            error: SERVER_MESSAGE,
            code: 'err.host_self.workflow_required',
            data: { permission: 'host_self.manage', target: 'app' },
          },
          { status: 403 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderWithProviders(<SaveHarness />);
    await user.click(screen.getByRole('button', { name: 'save' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Save refused');
    expect(alert).toHaveTextContent(SERVER_MESSAGE);
  });

  it('marks a workflow the server recorded as authorized, and nothing otherwise', () => {
    const { unmount } = renderWithProviders(<SelfHostAuthorizedBadge authorized />);
    expect(screen.getByText('Self-host authorized')).toBeInTheDocument();
    unmount();
    renderWithProviders(<SelfHostAuthorizedBadge authorized={false} />);
    expect(screen.queryByText('Self-host authorized')).not.toBeInTheDocument();
  });
});
