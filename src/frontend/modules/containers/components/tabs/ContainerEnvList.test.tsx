// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ContainerEnvList } from './ContainerEnvList';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';

const masked = [
  { key: 'DB_PASSWORD', value: ENV_REDACTION_MARKER },
  { key: 'MCHARBOR_SECRET', value: ENV_REDACTION_MARKER },
  { key: 'PLAIN', value: '1' },
];

describe('ContainerEnvList reveal', () => {
  const reveals = vi.fn();

  beforeEach(() => {
    reveals.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
    server.use(
      http.get('*/api/containers/c1', ({ request }) => {
        const url = new URL(request.url);
        reveals(url.searchParams.get('reveal'), url.searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          data: {
            Id: 'c1',
            Config: { Env: ['DB_PASSWORD=hunter2hunter2', `MCHARBOR_SECRET=${ENV_REDACTION_MARKER}`, 'PLAIN=1'] },
          },
        });
      }),
    );
  });

  it('shows the marker, never the value, before anything is revealed', () => {
    grantPermissions(['containers.view', 'containers.manage']);
    renderWithProviders(<ContainerEnvList containerId="c1" entries={masked} />);
    expect(screen.getAllByText(ENV_REDACTION_MARKER)).toHaveLength(2);
    expect(screen.queryByText('hunter2hunter2')).toBeNull();
    expect(reveals).not.toHaveBeenCalled();
  });

  it('renders Reveal disabled, naming containers.manage, and sends nothing when denied', async () => {
    grantPermissions(['containers.view']);
    const user = userEvent.setup();
    renderWithProviders(<ContainerEnvList containerId="c1" entries={masked} />);
    const button = screen.getByRole('button', { name: 'Reveal values' });
    expect(button).toBeDisabled();
    await user.click(button);
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the containers.manage permission')).not.toHaveLength(0);
    expect(reveals).not.toHaveBeenCalled();
    expect(screen.queryByText('hunter2hunter2')).toBeNull();
  });

  it('fetches with reveal=true and shows the values when containers.manage is held', async () => {
    grantPermissions(['containers.view', 'containers.manage']);
    const user = userEvent.setup();
    renderWithProviders(<ContainerEnvList containerId="c1" entries={masked} />);
    await user.click(screen.getByRole('button', { name: 'Reveal values' }));
    expect(await screen.findByText('hunter2hunter2')).toBeInTheDocument();
    expect(reveals).toHaveBeenCalledWith('true', 'env-1');
    expect(screen.getAllByText(ENV_REDACTION_MARKER)).toHaveLength(1);

    await user.click(screen.getByRole('button', { name: 'Hide values' }));
    await waitFor(() => expect(screen.queryByText('hunter2hunter2')).toBeNull());
  });

  it('offers no reveal when nothing was withheld', () => {
    grantPermissions(['containers.view', 'containers.manage']);
    renderWithProviders(<ContainerEnvList containerId="c1" entries={[{ key: 'PLAIN', value: '1' }]} />);
    expect(screen.queryByRole('button', { name: 'Reveal values' })).toBeNull();
  });
});
