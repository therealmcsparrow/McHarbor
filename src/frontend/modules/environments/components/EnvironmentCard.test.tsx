// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import type { EnvironmentListItem } from '@resources/hooks/useEnvironmentList';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { EnvironmentCard } from './EnvironmentCard';

const env: EnvironmentListItem = {
  id: 'env-off',
  name: 'Offline agent',
  connectionType: 'agent',
  agentStatus: 'disconnected',
  agentHostname: 'nas',
  dockerVersion: null,
  isDefault: false,
  isActive: true,
  lastConnected: null,
  scheduledUpdateCheckEnabled: false,
  automaticImagePruningEnabled: false,
  trackContainerEventsEnabled: false,
  collectContainerMetricsEnabled: false,
  highlightContainerChangesEnabled: false,
  dockerDiskUsageNotificationsEnabled: false,
  dockerDiskUsageThresholdPercent: 80,
  timezone: 'UTC',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  logRetentionDays: 30,
  metricRetentionHours: 24,
  autoUpdateWindowStart: '00:00',
  autoUpdateWindowEnd: '00:00',
  containerPruneEnabled: false,
  containerPruneStoppedDays: 7,
  imagePruneDanglingOnly: false,
};

function renderCard(environment: EnvironmentListItem) {
  return renderWithProviders(
    <MemoryRouter>
      <EnvironmentCard
        environment={environment}
        metricsEnabled
        onTest={() => {}}
        onInstall={() => {}}
        onRemove={() => {}}
      />
    </MemoryRouter>,
  );
}

describe('environment card disk strip', () => {
  it('still asks the server and shows the stale text when the environment is disconnected', async () => {
    grantPermissions(['host.view']);
    const seen: string[] = [];
    server.use(
      http.get('*/api/metrics/host/fs', ({ request }) => {
        seen.push(new URL(request.url).searchParams.get('env') ?? '');
        return HttpResponse.json({
          success: true,
          data: {
            environmentId: 'env-off',
            state: 'stale',
            reason: 'agent_offline',
            level: null,
            path: '/',
            totalBytes: 100,
            usedBytes: 50,
            percent: 50,
            lastSeen: new Date(Date.now() - 3 * 3600 * 1000).toISOString(),
            thresholds: { warning: 85, critical: 95 },
          },
        });
      }),
    );
    renderCard(env);
    expect(await screen.findByText(/No disk data, last seen/)).toBeInTheDocument();
    expect(seen).toEqual(['env-off']);
  });

  it('sends no request for an inactive environment', async () => {
    grantPermissions(['host.view']);
    const seen: string[] = [];
    server.use(
      http.get('*/api/metrics/host/fs', ({ request }) => {
        seen.push(new URL(request.url).searchParams.get('env') ?? '');
        return HttpResponse.json({ success: false, error: 'x' }, { status: 500 });
      }),
    );
    renderCard({ ...env, isActive: false });
    await screen.findByText('Offline agent');
    expect(screen.queryByText(/No disk data/)).not.toBeInTheDocument();
    expect(seen).toEqual([]);
  });
});
