// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { grantPermissions, resetPermissions } from '../../../test/permissions';
import { StatsPathNotice } from './StatsPathNotice';
import type { StatsPath } from '../hooks/useStatsPath';

function serve(path: StatsPath) {
  const seen = vi.fn();
  server.use(
    http.get('*/api/metrics/stats-path', ({ request }) => {
      seen(new URL(request.url).searchParams.get('env'));
      return HttpResponse.json({ success: true, data: path });
    }),
  );
  return seen;
}

describe('stats path notice', () => {
  afterEach(() => resetPermissions());

  it('names the push path and its interval', async () => {
    grantPermissions(['host.view']);
    const seen = serve({ mode: 'push', intervalSeconds: 10, lastBatchAt: new Date().toISOString() });
    renderWithProviders(<StatsPathNotice envId="env-1" />);
    expect(await screen.findByText('Pushed by the agent every 10 s')).toBeInTheDocument();
    expect(seen).toHaveBeenCalledWith('env-1');
  });

  it('says why an old agent is pulled, with the build it would need', async () => {
    grantPermissions(['host.view']);
    serve({
      mode: 'pull',
      reason: 'agent_too_old',
      agentVersion: '3.8.0',
      agentBuild: '',
      requiredVersion: '2026.09.01',
      requiredBuild: '202609251653',
    });
    renderWithProviders(<StatsPathNotice envId="env-1" />);
    expect(await screen.findByText('Pulled one container at a time')).toBeInTheDocument();
    expect(
      screen.getByText('Agent 3.8.0 (build -) cannot push stats. Build 202609251653 of 2026.09.01 or newer can.'),
    ).toBeInTheDocument();
  });

  it('does not ask for a path the viewer may not read', async () => {
    grantPermissions([]);
    const seen = serve({ mode: 'direct' });
    const { container } = renderWithProviders(<StatsPathNotice envId="env-1" />);
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(seen).not.toHaveBeenCalled();
    expect(container).toBeEmptyDOMElement();
  });
});
