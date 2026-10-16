// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WidgetCatalogSection } from './WidgetCatalogSection';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const clock = {
  key: 'clock',
  version: '2026.09.01',
  label: 'Clock',
  description: 'Shows the time',
  icon: 'IconClock',
  defaultSize: { w: 3, h: 2 },
  signed: true,
  installed: false,
};

describe('WidgetCatalogSection install control', () => {
  const installs = vi.fn();

  beforeEach(() => {
    installs.mockReset();
    server.use(
      http.get('*/api/widgets/catalog', () => HttpResponse.json({ success: true, data: [clock] })),
      http.post('*/api/widgets/catalog/clock/install', () => {
        installs();
        return HttpResponse.json({ success: true, data: { key: 'clock' } });
      }),
    );
  });

  it('renders Install disabled, naming dashboard_widgets.install, when the key is missing', async () => {
    grantPermissions(['widgets.view']);
    const user = userEvent.setup();
    renderWithProviders(<WidgetCatalogSection />);
    const button = await screen.findByRole('button', { name: 'Install' });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(installs).not.toHaveBeenCalled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the dashboard_widgets.install permission')).not.toHaveLength(0);
  });

  it('installs exactly once when the key is held', async () => {
    grantPermissions(['widgets.view', 'dashboard_widgets.install']);
    const user = userEvent.setup();
    renderWithProviders(<WidgetCatalogSection />);
    await user.click(await screen.findByRole('button', { name: 'Install' }));
    await waitFor(() => expect(installs).toHaveBeenCalledTimes(1));
  });

  it("shows the server's own refusal message in full", async () => {
    const message = 'The widget bundle is not signed by a trusted key, so it will not be installed or loaded';
    server.use(
      http.post('*/api/widgets/catalog/clock/install', () =>
        HttpResponse.json({ success: false, error: message, code: 'err.widget.signature_invalid' }, { status: 400 }),
      ),
    );
    grantPermissions(['widgets.view', 'dashboard_widgets.install']);
    const user = userEvent.setup();
    renderWithProviders(<WidgetCatalogSection />);
    await user.click(await screen.findByRole('button', { name: 'Install' }));
    const shown = await screen.findByText((_, el) => el?.tagName === 'P' && (el.textContent ?? '').includes(message));
    expect(shown.textContent).toContain('Clock could not be installed');
  });

  it('keeps Install disabled for an entry no trust key verifies, even with the key', async () => {
    server.use(
      http.get('*/api/widgets/catalog', () =>
        HttpResponse.json({ success: true, data: [{ ...clock, signed: false }] }),
      ),
    );
    grantPermissions(['widgets.view', 'dashboard_widgets.install']);
    renderWithProviders(<WidgetCatalogSection />);
    expect(await screen.findByText('Not signed by a trusted key')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Install' })).toBeDisabled();
  });
});
