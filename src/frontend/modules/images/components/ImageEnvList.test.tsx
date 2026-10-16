// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ImageEnvList } from './ImageEnvList';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const masked = [`DB_PASSWORD=${ENV_REDACTION_MARKER}`, `COPIED=${ENV_REDACTION_MARKER}`, 'PATH=/usr/bin'];

describe('ImageEnvList reveal', () => {
  const reveals = vi.fn();

  beforeEach(() => {
    reveals.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
    server.use(
      http.get('*/api/images/img1', ({ request }) => {
        const url = new URL(request.url);
        reveals(url.searchParams.get('reveal'), url.searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          data: {
            Id: 'img1',
            Config: { Env: ['DB_PASSWORD=hunter2hunter2', `COPIED=${ENV_REDACTION_MARKER}`, 'PATH=/usr/bin'] },
          },
        });
      }),
    );
  });

  it('shows the marker, never the value, before anything is revealed', () => {
    grantPermissions(['images.view', 'images.manage']);
    renderWithProviders(<ImageEnvList imageId="img1" env={masked} />);
    expect(screen.getAllByText(ENV_REDACTION_MARKER)).toHaveLength(2);
    expect(screen.queryByText('hunter2hunter2')).toBeNull();
    expect(reveals).not.toHaveBeenCalled();
  });

  it('renders Reveal disabled, naming images.manage, and sends nothing when denied', async () => {
    grantPermissions(['images.view']);
    const user = userEvent.setup();
    renderWithProviders(<ImageEnvList imageId="img1" env={masked} />);
    const button = screen.getByRole('button', { name: 'Reveal values' });
    expect(button).toBeDisabled();
    await user.click(button);
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the images.manage permission')).not.toHaveLength(0);
    expect(reveals).not.toHaveBeenCalled();
  });

  it('fetches with reveal=true and shows the values when images.manage is held', async () => {
    grantPermissions(['images.view', 'images.manage']);
    const user = userEvent.setup();
    renderWithProviders(<ImageEnvList imageId="img1" env={masked} />);
    await user.click(screen.getByRole('button', { name: 'Reveal values' }));
    expect(await screen.findByText('hunter2hunter2')).toBeInTheDocument();
    expect(reveals).toHaveBeenCalledWith('true', 'env-1');
    expect(screen.getAllByText(ENV_REDACTION_MARKER)).toHaveLength(1);

    await user.click(screen.getByRole('button', { name: 'Hide values' }));
    await waitFor(() => expect(screen.queryByText('hunter2hunter2')).toBeNull());
  });

  it('offers no reveal when nothing was withheld', () => {
    grantPermissions(['images.view', 'images.manage']);
    renderWithProviders(<ImageEnvList imageId="img1" env={['PATH=/usr/bin']} />);
    expect(screen.queryByRole('button', { name: 'Reveal values' })).toBeNull();
  });
});
