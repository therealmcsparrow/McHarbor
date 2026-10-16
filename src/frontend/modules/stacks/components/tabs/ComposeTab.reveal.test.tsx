// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ComposeTab } from './ComposeTab';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

const masked = `services:\n  web:\n    environment:\n      DB_PASSWORD: ${ENV_REDACTION_MARKER}\n      MODE: blue\n`;
const revealed = 'services:\n  web:\n    environment:\n      DB_PASSWORD: hunter2hunter2\n      MODE: blue\n';

describe('ComposeTab reveal', () => {
  const reveals = vi.fn();

  beforeEach(() => {
    reveals.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
    server.use(
      http.get('*/api/stacks/web/compose', ({ request }) => {
        const reveal = new URL(request.url).searchParams.get('reveal');
        reveals(reveal);
        return HttpResponse.json({ success: true, data: { content: reveal === 'true' ? revealed : masked } });
      }),
    );
  });

  it('renders Reveal disabled, naming stacks.manage, and never asks for the values when denied', async () => {
    grantPermissions(['stacks.view']);
    const user = userEvent.setup();
    renderWithProviders(<ComposeTab stackName="web" isManaged readOnly />);
    const button = await screen.findByRole('button', { name: 'Reveal values' });
    expect(button).toBeDisabled();
    await user.click(button);
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the stacks.manage permission')).not.toHaveLength(0);
    expect(reveals).not.toHaveBeenCalledWith('true');
    expect(screen.queryByText(/hunter2hunter2/)).toBeNull();
  });

  it('shows the values after a reveal when stacks.manage is held, and hides them again', async () => {
    grantPermissions(['stacks.view', 'stacks.manage']);
    const user = userEvent.setup();
    renderWithProviders(<ComposeTab stackName="web" isManaged readOnly />);
    await user.click(await screen.findByRole('button', { name: 'Reveal values' }));
    expect(await screen.findByText(/hunter2hunter2/)).toBeInTheDocument();
    expect(reveals).toHaveBeenCalledWith('true');
    await user.click(screen.getByRole('button', { name: 'Hide values' }));
    await waitFor(() => expect(screen.queryByText(/hunter2hunter2/)).toBeNull());
  });
});
