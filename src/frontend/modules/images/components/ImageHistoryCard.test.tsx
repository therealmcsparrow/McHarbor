// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ImageHistoryItem } from '@core/types/docker';
import { ENV_REDACTION_MARKER } from '@resources/utils/env-redaction';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ImageHistoryCard } from './ImageHistoryCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

function layer(id: string, createdBy: string): ImageHistoryItem {
  return { Id: id, Created: 1_700_000_000, CreatedBy: createdBy, Size: 0, Tags: null, Comment: '' } as ImageHistoryItem;
}

const masked = [layer('a', `ENV API_TOKEN=${ENV_REDACTION_MARKER}`), layer('b', 'CMD ["app"]')];

describe('ImageHistoryCard reveal', () => {
  const reveals = vi.fn();

  beforeEach(() => {
    reveals.mockReset();
    useEnvironmentStore.setState({ currentId: 'env-1' });
    server.use(
      http.get('*/api/images/img1/history', ({ request }) => {
        const url = new URL(request.url);
        reveals(url.searchParams.get('reveal'), url.searchParams.get('env'));
        return HttpResponse.json({
          success: true,
          data: [layer('a', 'ENV API_TOKEN=tok-abcdef'), layer('b', 'CMD ["app"]')],
        });
      }),
    );
  });

  it('renders Reveal disabled, naming images.manage, and sends nothing when denied', async () => {
    grantPermissions(['images.view']);
    const user = userEvent.setup();
    renderWithProviders(<ImageHistoryCard imageId="img1" history={masked} />);
    expect(screen.getByText(`ENV API_TOKEN=${ENV_REDACTION_MARKER}`)).toBeInTheDocument();
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
    renderWithProviders(<ImageHistoryCard imageId="img1" history={masked} />);
    await user.click(screen.getByRole('button', { name: 'Reveal values' }));
    expect(await screen.findByText('ENV API_TOKEN=tok-abcdef')).toBeInTheDocument();
    expect(reveals).toHaveBeenCalledWith('true', 'env-1');

    await user.click(screen.getByRole('button', { name: 'Hide values' }));
    await waitFor(() => expect(screen.queryByText('ENV API_TOKEN=tok-abcdef')).toBeNull());
  });

  it('offers no reveal when nothing was withheld', () => {
    grantPermissions(['images.view', 'images.manage']);
    renderWithProviders(<ImageHistoryCard imageId="img1" history={[layer('b', 'CMD ["app"]')]} />);
    expect(screen.queryByRole('button', { name: 'Reveal values' })).toBeNull();
  });
});
