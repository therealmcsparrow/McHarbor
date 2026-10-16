// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from 'i18next';
import { clearPluginCapabilities, givePluginCapabilities } from '../../../test/plugin-capabilities';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { EnvironmentEstateRow } from './EnvironmentEstateRow';

const base = {
  plugin: 'acme-widgets',
  kind: 'estate-source',
  labelNs: 'widgets',
  icon: 'server',
};
const filtered = {
  ...base,
  metric: 'widgets',
  path: '/api/widgets',
  environmentField: 'envId',
  labelKey: 'estate.widgets',
  runningField: 'state',
  runningValues: ['up'],
  link: '/plugins/acme-widgets/{environmentId}',
};
const scoped = {
  ...base,
  plugin: 'acme-gizmos',
  labelNs: 'gizmos',
  metric: 'gizmos',
  path: '/api/gizmos',
  envScoped: true,
  labelKey: 'estate.gizmos',
};

beforeAll(() => {
  i18n.addResourceBundle('en', 'widgets', { estate: { widgets: 'Widgets' } }, true, true);
  i18n.addResourceBundle('en', 'gizmos', { estate: { gizmos: 'Gizmos' } }, true, true);
});

function renderRow() {
  return renderWithProviders(
    <MemoryRouter>
      <EnvironmentEstateRow environmentId="env-1" enabled />
    </MemoryRouter>,
  );
}

describe('environment card estate row', () => {
  afterEach(() => clearPluginCapabilities());

  it('shows running over total for the objects of this environment only, linked to the plugin', async () => {
    givePluginCapabilities([filtered]);
    server.use(
      http.get('*/api/widgets', () =>
        HttpResponse.json({
          success: true,
          data: [
            { envId: 'env-1', state: 'up' },
            { envId: 'env-1', state: 'down' },
            { envId: 'env-1', state: 'up' },
            { envId: 'env-2', state: 'up' },
          ],
        }),
      ),
    );
    renderRow();
    expect(await screen.findByText('2/3')).toBeInTheDocument();
    expect(screen.getByText('Widgets')).toBeInTheDocument();
    expect(screen.getByRole('link')).toHaveAttribute('href', '/plugins/acme-widgets/env-1');
  });

  it('asks an env scoped source for this environment and shows the plain total', async () => {
    givePluginCapabilities([scoped]);
    const seen = vi.fn();
    server.use(
      http.get('*/api/gizmos', ({ request }) => {
        seen(new URL(request.url).searchParams.get('env'));
        return HttpResponse.json({ success: true, data: [{ state: 'a' }, { state: 'b' }] });
      }),
    );
    renderRow();
    expect(await screen.findByText('2')).toBeInTheDocument();
    expect(screen.getByText('Gizmos')).toBeInTheDocument();
    expect(seen).toHaveBeenCalledWith('env-1');
  });

  it('leaves out a source the user may not read instead of showing zero', async () => {
    givePluginCapabilities([filtered, scoped]);
    server.use(
      http.get('*/api/widgets', () => HttpResponse.json({ success: false, code: 'forbidden' }, { status: 403 })),
      http.get('*/api/gizmos', () => HttpResponse.json({ success: true, data: [{}] })),
    );
    renderRow();
    expect(await screen.findByText('Gizmos')).toBeInTheDocument();
    expect(screen.queryByText('Widgets')).not.toBeInTheDocument();
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('renders nothing for a source that does not declare a label', async () => {
    const dashboardOnly = { ...filtered, labelKey: undefined, labelNs: undefined, icon: undefined };
    givePluginCapabilities([dashboardOnly]);
    const listed = vi.fn();
    server.use(
      http.get('*/api/widgets', () => {
        listed();
        return HttpResponse.json({ success: true, data: [] });
      }),
    );
    const { container } = renderRow();
    expect(container).toBeEmptyDOMElement();
    expect(listed).not.toHaveBeenCalled();
  });
});
